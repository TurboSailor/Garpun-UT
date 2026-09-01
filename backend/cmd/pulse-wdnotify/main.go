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
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"pulse/backend/internal/fdnotify"
	"pulse/backend/internal/uxbridge"
)

func main() {
	var (
		waydroid = flag.String("waydroid", "192.168.240.112:5555", "Waydroid adb endpoint")
		interval = flag.Duration("interval", 2*time.Second, "how often to poll the container")
		debug    = flag.Bool("debug", false, "verbose logging")
		sudoPass = flag.String("sudo-pass", "", "password for the privileged `waydroid shell` fallback")
	)
	flag.Parse()

	level := slog.LevelInfo
	if *debug || os.Getenv("PULSE_DEBUG") != "" {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	if err := run(log, *waydroid, *interval, *sudoPass); err != nil {
		log.Error("pulse-wdnotify exited", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger, addr string, interval time.Duration, sudoPass string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shade, err := fdnotify.New()
	if err != nil {
		return err
	}
	defer shade.Disconnect()

	if name, vendor, version, err := shade.ServerInfo(); err == nil {
		log.Info("notification server", "name", name, "vendor", vendor, "version", version)
	}

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

	r := &relay{log: log, shade: shade, posted: map[int32]uint32{}}
	log.Info("relaying waydroid notifications into the shade", "addr", addr, "interval", interval)

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

// relay tracks which shade entry belongs to which bridge notification so an
// update replaces the old bubble instead of stacking a new one, and a removal
// in Android closes the entry here too.
type relay struct {
	log   *slog.Logger
	shade *fdnotify.Client

	mu     sync.Mutex
	posted map[int32]uint32
}

func (r *relay) handle(n uxbridge.Notification) {
	if n.Source != uxbridge.SourceWaydroid {
		return
	}

	r.mu.Lock()
	serverID, seen := r.posted[n.ID]
	r.mu.Unlock()

	if n.Removed {
		if !seen {
			return
		}
		if err := r.shade.Close(serverID); err != nil {
			r.log.Debug("could not close shade entry", "err", err, "id", serverID)
		}
		r.mu.Lock()
		delete(r.posted, n.ID)
		r.mu.Unlock()
		r.log.Info("notification retracted", "app", n.AppName, "shadeId", serverID)
		return
	}

	appName := n.AppName
	if appName == "" {
		appName = n.AppID
	}
	id, err := r.shade.Post(fdnotify.Notification{
		AppName:    appName,
		AppID:      n.AppID,
		Source:     uxbridge.SourceWaydroid,
		Summary:    n.Title,
		Body:       n.Body,
		ReplacesID: serverID,
	})
	if err != nil {
		r.log.Warn("could not post to the shade", "err", err, "app", appName)
		return
	}

	r.mu.Lock()
	r.posted[n.ID] = id
	r.mu.Unlock()
	r.log.Info("notification relayed", "app", appName, "title", n.Title, "shadeId", id)
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
