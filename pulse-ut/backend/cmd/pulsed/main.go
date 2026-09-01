// Command pulsed is the Pulse background service: it owns the Bluetooth link
// to the watch, the local database and the HTTP API the QML front end uses.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"pulse/backend/internal/api"
	"pulse/backend/internal/daemon"
	"pulse/backend/internal/garmin"
	"pulse/backend/internal/garminhttp"
	"pulse/backend/internal/gfdi"
	"pulse/backend/internal/store"
	"pulse/backend/internal/uxbridge"
	"pulse/backend/internal/weather"
)

func main() {
	var (
		addr     = flag.String("addr", "127.0.0.1:21830", "HTTP listen address")
		dbPath   = flag.String("db", defaultDBPath(), "database file")
		debug    = flag.Bool("debug", false, "verbose logging")
		noWatch  = flag.Bool("no-bluetooth", false, "run the API without touching Bluetooth")
		waydroid = flag.String("waydroid", "192.168.240.112:5555", "Waydroid adb endpoint for notification capture")
	)
	flag.Parse()

	level := slog.LevelInfo
	if *debug || os.Getenv("PULSE_DEBUG") != "" {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	if err := run(log, *addr, *dbPath, *waydroid, *noWatch); err != nil {
		log.Error("pulsed exited", "err", err)
		os.Exit(1)
	}
}

func defaultDBPath() string {
	if v := os.Getenv("PULSE_DB"); v != "" {
		return v
	}
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "pulse.db"
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "pulse", "pulse.db")
}

func run(log *slog.Logger, addr, dbPath, waydroidAddr string, noBluetooth bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	log.Info("database ready", "path", db.Path())

	// Phone-side integrations. Each one is optional: a missing session bus or
	// a stopped Waydroid container must not keep the watch from syncing.
	bridge, err := uxbridge.New(log, uxbridge.Options{
		EnableFreedesktop:    true,
		EnableWaydroid:       db.Setting("notifyWaydroid", "true") != "false",
		WaydroidAddr:         waydroidAddr,
		WaydroidPollInterval: 2 * time.Second,
	})
	if err != nil {
		log.Warn("notification bridge unavailable", "err", err)
		bridge = nil
	}

	weatherSvc := weather.New(log, weather.Options{})

	// The protobuf handler needs to answer asynchronously, and its battery
	// callback needs the manager, which does not exist yet: capture it.
	var mgr *daemon.Manager
	protoHandler := garminhttp.New(log, garminhttp.Options{
		Weather: weatherSvc,
		OnBattery: func(level int32) {
			if mgr != nil {
				mgr.OnBatteryLevel(level)
			}
		},
	})

	deps := daemon.Deps{
		Weather:  weatherSvc.FitPayload,
		Protobuf: protoHandler.Handle,
		SetProtobufSender: func(send func(uint16, []byte)) {
			protoHandler.SetSender(send)
		},
	}
	if bridge != nil {
		deps.NotificationContent = bridge.Lookup
		deps.AppName = bridge.AppName
		deps.MusicMetadata = bridge.MusicMetadata
		deps.MusicCommand = bridge.MusicCommand
		deps.AcceptCall = bridge.AcceptCall
		deps.RejectCall = bridge.RejectCall
		deps.Notifications = adaptNotifications(ctx, bridge)
		defer bridge.Close()
	}

	var feed api.NotificationFeed
	if bridge != nil {
		feed = notificationFeed{bridge}
	}

	if !noBluetooth {
		mgr, err = daemon.New(ctx, db, log, deps)
		if err != nil {
			return fmt.Errorf("bluetooth unavailable: %w", err)
		}
		defer mgr.Close()

		// Reconnect to the last known watch on startup so the app is useful
		// straight after boot without any tapping.
		if devices, err := db.Devices(); err == nil && len(devices) > 0 {
			go func(address string) {
				if err := mgr.Connect(address); err != nil {
					log.Info("initial connect failed, will retry", "err", err)
				}
			}(devices[0].Address)
		}
	} else {
		return errors.New("pulsed: -no-bluetooth leaves nothing to serve")
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           api.New(db, mgr, log, feed).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("pulsed: listen %s: %w", addr, err)
	}
	log.Info("api listening", "addr", ln.Addr().String())

	errCh := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutting down")
	case err := <-errCh:
		return err
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// adaptNotifications converts bridge notifications into what the daemon feeds
// to the watch.
func adaptNotifications(ctx context.Context, b *uxbridge.Bridge) <-chan daemon.NotifyEvent {
	out := make(chan daemon.NotifyEvent, 32)
	go func() {
		defer close(out)
		src := b.Notifications()
		for {
			select {
			case <-ctx.Done():
				return
			case n, ok := <-src:
				if !ok {
					return
				}
				ev := daemon.NotifyEvent{
					Removed: n.Removed,
					Content: garmin.NotificationContent{
						ID:            n.ID,
						AppIdentifier: n.AppID,
						Title:         n.Title,
						Subtitle:      n.AppName,
						Message:       n.Body,
						Date:          time.UnixMilli(n.TsMs),
						Category:      n.Category,
						Actions:       n.Actions,
					},
				}
				if ev.Content.Category == gfdi.CategoryIncomingCall && len(ev.Content.Actions) == 0 {
					ev.Content.Actions = []garmin.NotificationAction{
						{Code: gfdi.ActionAcceptIncomingCall, Label: "Answer"},
						{Code: gfdi.ActionRejectIncomingCall, Label: "Decline"},
					}
				}
				select {
				case out <- ev:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out
}

// notificationFeed exposes the bridge history to the API.
type notificationFeed struct{ b *uxbridge.Bridge }

func (f notificationFeed) RecentJSON(limit int) any {
	items := f.b.Recent(limit)
	if items == nil {
		return []any{}
	}
	return items
}
