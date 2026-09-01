// Command pulse-wdnotify relays Android notifications out of the Waydroid
// container into the Ubuntu Touch notification shade.
//
// Waydroid runs a whole Android userspace whose notifications never reach
// Lomiri: the container has its own notification manager and nothing bridges
// the two. This service polls Android's notification service and re-posts what
// it finds through org.freedesktop.Notifications, which on Ubuntu Touch is
// owned by Lomiri itself. The result is a single shade holding both native and
// Android notifications.
//
// It is deliberately independent of pulsed. Running it is useful on its own,
// with or without a watch; pulsed simply observes the same shade and forwards
// whatever lands there.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"pulse/backend/internal/postal"
	"pulse/backend/internal/uxbridge"
)

// appID is this click package's push identity: <package>_<hook>. The push
// service refuses to deliver for anything it cannot resolve to an installed
// application declaring a push-helper.
const appID = "cc.zachy.pulse_pulse"

func main() {
	var (
		waydroid   = flag.String("waydroid", "192.168.240.112:5555", "Waydroid adb endpoint")
		interval   = flag.Duration("interval", 2*time.Second, "how often to poll the container")
		pulsedAddr = flag.String("pulsed", "127.0.0.1:21830", "pulsed API address, for mirroring to the watch")
		debug      = flag.Bool("debug", false, "verbose logging")
		sudoPass   = flag.String("sudo-pass", "", "password for the privileged `waydroid shell` fallback")
	)
	flag.Parse()

	level := slog.LevelInfo
	if *debug || os.Getenv("PULSE_DEBUG") != "" {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	if err := run(log, *waydroid, *pulsedAddr, *interval, *sudoPass); err != nil {
		log.Error("pulse-wdnotify exited", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger, addr, pulsedAddr string, interval time.Duration, sudoPass string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shade, err := postal.New(appID)
	if err != nil {
		return err
	}
	defer shade.Disconnect()

	bridge, err := uxbridge.New(log, uxbridge.Options{
		EnableWaydroid:       true,
		WaydroidAddr:         addr,
		WaydroidPollInterval: interval,
		PrivilegedRunner:     privilegedRunner(sudoPass),
	})
	if err != nil {
		return err
	}
	defer bridge.Close()

	r := &relay{
		log:    log,
		shade:  shade,
		pulsed: &pulsedClient{log: log, addr: pulsedAddr, http: &http.Client{Timeout: 3 * time.Second}},
		tags:   map[int32]string{},
	}
	log.Info("relaying waydroid notifications into the notification list",
		"addr", addr, "interval", interval, "postalPath", shade.Path())

	for {
		select {
		case <-ctx.Done():
			log.Info("shutting down")
			return nil
		case n, ok := <-bridge.Notifications():
			if !ok {
				return nil
			}
			r.handle(n)
		}
	}
}

// relay turns each Android notification into a native Ubuntu Touch one and
// tells pulsed about it.
//
// Two sinks, because they are genuinely different systems. The push service
// owns the phone's notification list; the watch is fed by pulsed, which
// watches the session bus and therefore cannot see anything the push service
// delivered. Posting to both keeps one poller and no duplicates.
type relay struct {
	log    *slog.Logger
	shade  *postal.Client
	pulsed *pulsedClient

	mu   sync.Mutex
	tags map[int32]string
}

func (r *relay) handle(n uxbridge.Notification) {
	if n.Source != uxbridge.SourceWaydroid {
		return
	}

	appName := n.AppName
	if appName == "" {
		appName = n.AppID
	}
	key := n.AppID + ":" + strconv.FormatInt(int64(n.ID), 10)

	r.mu.Lock()
	tag, seen := r.tags[n.ID]
	r.mu.Unlock()

	if n.Removed {
		if seen {
			if _, err := r.shade.ClearPersistent(tag); err != nil {
				r.log.Debug("could not clear the notification list entry", "err", err, "tag", tag)
			}
			r.mu.Lock()
			delete(r.tags, n.ID)
			r.mu.Unlock()
		}
		r.pulsed.inject(key, n, appName, true)
		r.log.Info("notification retracted", "app", appName)
		return
	}

	if !seen {
		tag = "wd-" + strconv.FormatInt(int64(n.ID), 10)
	}
	// Posting the same tag again replaces the entry rather than stacking one.
	err := r.shade.Post(tag, postal.Card{
		Summary: appName,
		Body:    summaryBody(n),
		Popup:   true,
		Persist: true,
	})
	if err != nil {
		r.log.Warn("could not post to the notification list", "err", err, "app", appName)
	} else {
		r.mu.Lock()
		r.tags[n.ID] = tag
		r.mu.Unlock()
	}

	r.pulsed.inject(key, n, appName, false)
	r.log.Info("notification relayed", "app", appName, "title", n.Title, "tag", tag)
}

// summaryBody keeps the Android title visible: the card summary is the app
// name, so the title has to lead the body or it is lost.
func summaryBody(n uxbridge.Notification) string {
	switch {
	case n.Title != "" && n.Body != "":
		return n.Title + ": " + n.Body
	case n.Title != "":
		return n.Title
	default:
		return n.Body
	}
}

// pulsedClient forwards to the local daemon so the watch mirrors the shade.
// pulsed not running is normal — the relay still fills the phone's list.
type pulsedClient struct {
	log  *slog.Logger
	addr string
	http *http.Client
}

func (p *pulsedClient) inject(key string, n uxbridge.Notification, appName string, removed bool) {
	if p == nil {
		return
	}
	payload, err := json.Marshal(map[string]any{
		"key":     key,
		"source":  uxbridge.SourceWaydroid,
		"appId":   n.AppID,
		"appName": appName,
		"title":   n.Title,
		"body":    n.Body,
		"removed": removed,
	})
	if err != nil {
		return
	}
	resp, err := p.http.Post("http://"+p.addr+"/api/notifications", "application/json", bytes.NewReader(payload))
	if err != nil {
		p.log.Debug("pulsed unreachable, watch will not mirror this one", "err", err)
		return
	}
	resp.Body.Close()
}

// privilegedRunner is the fallback into the container for when its adbd
// demands authentication. Without a password there is simply no fallback.
func privilegedRunner(pass string) func(context.Context, ...string) ([]byte, error) {
	if pass == "" {
		pass = os.Getenv("PULSE_SUDO_PASS")
	}
	if pass == "" {
		return nil
	}
	return func(ctx context.Context, args ...string) ([]byte, error) {
		full := append([]string{"-S", "waydroid"}, args...)
		cmd := exec.CommandContext(ctx, "sudo", full...)
		cmd.Stdin = strings.NewReader(pass + "\n")
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("sudo waydroid: %w", err)
		}
		return out, nil
	}
}
