// Command pulsectl is the on-device diagnostic driver for the Pulse backend.
// It exercises the same BLE, GFDI and sync code the daemon uses, but prints
// everything to the terminal so a session can be debugged over adb.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"pulse/backend/internal/ble"
	"pulse/backend/internal/garmin"
	"pulse/backend/internal/importer"
	"pulse/backend/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	level := slog.LevelInfo
	if os.Getenv("PULSE_DEBUG") != "" {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var err error
	switch os.Args[1] {
	case "scan":
		err = cmdScan(ctx, log, os.Args[2:])
	case "pair":
		err = cmdPair(ctx, log, os.Args[2:])
	case "connect":
		err = cmdConnect(ctx, log, os.Args[2:])
	case "info":
		err = cmdInfo(ctx, log)
	case "reimport":
		err = cmdReimport(log, os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  pulsectl scan [-t 20s]              discover BLE devices, flag Garmin watches
  pulsectl pair <MAC>                 bond with a watch (watch must be in pairing mode)
  pulsectl connect <MAC> [-sync] [-out DIR]
  pulsectl reimport [-db PATH]        rebuild samples from stored FIT files
  pulsectl info                       adapter status`)
	os.Exit(2)
}

func openAdapter(ctx context.Context) (*ble.Bus, *ble.Adapter, error) {
	bus, err := ble.Dial()
	if err != nil {
		return nil, nil, err
	}
	ad, err := bus.DefaultAdapter()
	if err != nil {
		return nil, nil, err
	}
	powerCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := ad.SetPowered(powerCtx, true); err != nil {
		return nil, nil, fmt.Errorf("power on adapter: %w", err)
	}
	return bus, ad, nil
}

func cmdInfo(ctx context.Context, log *slog.Logger) error {
	_, ad, err := openAdapter(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("adapter %s powered=%v\n", ad.Address(), ad.Powered())
	for _, d := range ad.Devices() {
		fmt.Printf("  %s paired=%-5v connected=%-5v %s\n", d.Address, d.Paired, d.Conn, d.Name)
	}
	return nil
}

func cmdScan(ctx context.Context, log *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	dur := fs.Duration("t", 20*time.Second, "scan duration")
	fs.Parse(args)

	_, ad, err := openAdapter(ctx)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	err = ad.Discover(ctx, *dur, func(d ble.DeviceInfo) {
		if seen[d.Address] {
			return
		}
		seen[d.Address] = true
		tag := ""
		if isGarmin(d) {
			tag = "  <-- GARMIN"
		}
		fmt.Printf("%s  rssi=%-5d paired=%-5v %s%s\n", d.Address, d.RSSI, d.Paired, d.Name, tag)
	})
	if err != nil {
		return err
	}
	fmt.Printf("\n%d devices\n", len(seen))
	return nil
}

var garminNameHints = []string{
	"garmin", "forerunner", "fenix", "instinct", "venu", "vivo", "vívo", "epix",
	"enduro", "tactix", "quatix", "descent", "lily", "swim", "marq", "approach",
	"edge", "hrm", "index", "d2 ", "cirqa",
}

func isGarmin(d ble.DeviceInfo) bool {
	name := strings.ToLower(d.Name + " " + d.Alias)
	for _, h := range garminNameHints {
		if strings.Contains(name, h) {
			return true
		}
	}
	for _, u := range d.UUIDs {
		u = strings.ToLower(u)
		if strings.HasPrefix(u, "6a4e") || strings.HasPrefix(u, "9b012401") {
			return true
		}
	}
	return false
}

// consoleUI answers BlueZ pairing callbacks from the terminal.
type consoleUI struct{}

func (consoleUI) RequestPasskey(device string) (uint32, error) {
	fmt.Printf("\n*** %s wants a passkey. Type the 6 digits shown on the watch: ", device)
	var key uint32
	if _, err := fmt.Scanf("%d", &key); err != nil {
		return 0, err
	}
	return key, nil
}

func (consoleUI) RequestConfirmation(device string, passkey uint32) error {
	fmt.Printf("\n*** %s passkey %06d - confirm on the watch, then press enter", device, passkey)
	fmt.Scanln()
	return nil
}

func (consoleUI) DisplayPasskey(device string, passkey uint32, entered uint16) {
	fmt.Printf("\n*** %s passkey: %06d (entered %d)\n", device, passkey, entered)
}

func (consoleUI) Cancel() { fmt.Println("\n*** pairing cancelled") }

func cmdPair(ctx context.Context, log *slog.Logger, args []string) error {
	if len(args) < 1 {
		usage()
	}
	addr := args[0]
	bus, ad, err := openAdapter(ctx)
	if err != nil {
		return err
	}

	agent, err := ble.NewAgent(bus, consoleUI{})
	if err != nil {
		return err
	}
	if err := agent.Register("KeyboardDisplay"); err != nil {
		return err
	}
	defer agent.Unregister()

	dev, err := ad.Device(addr)
	if err != nil {
		fmt.Println("device unknown, scanning for it...")
		scanCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		found := make(chan struct{})
		var once bool
		go ad.Discover(scanCtx, 30*time.Second, func(d ble.DeviceInfo) {
			if strings.EqualFold(d.Address, addr) && !once {
				once = true
				close(found)
			}
		})
		select {
		case <-found:
		case <-scanCtx.Done():
		}
		cancel()
		dev, err = ad.Device(addr)
		if err != nil {
			return err
		}
	}

	pairCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	if err := dev.Pair(pairCtx); err != nil {
		return err
	}
	if err := dev.SetTrusted(true); err != nil {
		log.Warn("could not mark device trusted", "err", err)
	}
	fmt.Printf("paired with %s (%s)\n", dev.Address(), dev.Name())
	return nil
}

func cmdConnect(ctx context.Context, log *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("connect", flag.ExitOnError)
	doSync := fs.Bool("sync", false, "request a file sync once initialised")
	outDir := fs.String("out", "", "directory to write downloaded files into")
	first := fs.Bool("first", false, "send first-connect pairing events")
	if len(args) < 1 {
		usage()
	}
	addr := args[0]
	fs.Parse(args[1:])

	bus, ad, err := openAdapter(ctx)
	if err != nil {
		return err
	}
	agent, err := ble.NewAgent(bus, consoleUI{})
	if err != nil {
		return err
	}
	if err := agent.Register("KeyboardDisplay"); err != nil {
		log.Warn("pairing agent not registered", "err", err)
	} else {
		defer agent.Unregister()
	}

	dev, err := ad.Device(addr)
	if err != nil {
		return err
	}

	connCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	fmt.Printf("connecting to %s...\n", addr)
	if err := dev.Connect(connCtx); err != nil {
		return err
	}
	fmt.Printf("connected: %s\n", dev.Name())
	for _, u := range dev.ServiceUUIDs() {
		fmt.Println("  service", u)
	}

	trCtx, trCancel := context.WithTimeout(ctx, 30*time.Second)
	tr, err := garmin.OpenTransport(trCtx, dev, log)
	trCancel()
	if err != nil {
		return err
	}
	fmt.Println("transport:", tr.Version())

	if *outDir != "" {
		if err := os.MkdirAll(*outDir, 0o755); err != nil {
			return err
		}
	}

	sess := garmin.NewSession(ctx, tr, garmin.Options{
		PhoneName:    "Pulse",
		Manufacturer: "Ubuntu Touch",
		Model:        "Pulse",
		SyncTime:     true,
		FirstConnect: *first,
	}, log)
	defer sess.Close()

	sess.Hooks.FileDownloaded = func(entry garmin.DirectoryEntry, data []byte) error {
		if *outDir == "" {
			return nil
		}
		path := filepath.Join(*outDir, entry.Name())
		return os.WriteFile(path, data, 0o644)
	}

	syncRequested := false
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-sess.Events():
			if !ok {
				return nil
			}
			b, _ := json.Marshal(ev.Data)
			fmt.Printf("[%s] %s\n", ev.Kind, string(b))
			if ev.Kind == garmin.EventInitialized && *doSync && !syncRequested {
				syncRequested = true
				fmt.Println(">>> requesting sync")
				sess.StartSync()
			}
			if ev.Kind == garmin.EventSyncFinished && *doSync {
				fmt.Println(">>> sync finished")
				return nil
			}
			if ev.Kind == garmin.EventDisconnected {
				return fmt.Errorf("watch disconnected")
			}
		}
	}
}

// cmdReimport rebuilds every derived table from the FIT blobs already in the
// database. Used after an importer fix so the dashboard is corrected without
// pulling the files off the watch again.
func cmdReimport(log *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("reimport", flag.ExitOnError)
	dbPath := fs.String("db", defaultDBPath(), "database file")
	fs.Parse(args)

	db, err := store.Open(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	fmt.Println("database:", db.Path())

	devices, err := db.Devices()
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		return fmt.Errorf("no device rows: nothing to reimport")
	}

	im := importer.New(db, log)
	for _, dev := range devices {
		files, err := db.FitFiles(dev.ID)
		if err != nil {
			return err
		}
		fmt.Printf("%s (%s): %d stored files\n", dev.Name, dev.Address, len(files))
		if err := db.ResetDerived(dev.ID); err != nil {
			return err
		}
		fmt.Println("  derived tables cleared")

		var ok, failed int
		totals := map[string]int{}
		for _, f := range files {
			res, err := im.Import(dev.ID, uint8(f.SubType), f.Data)
			if err != nil {
				failed++
				log.Warn("reimport failed", "file", f.FileNumber, "err", err)
				continue
			}
			ok++
			totals[res.FileType] += res.Activity + res.Stress + res.SleepStages
		}
		fmt.Printf("  reimported %d files (%d failed)\n", ok, failed)
		for kind, n := range totals {
			fmt.Printf("    %-12s %d rows\n", kind, n)
		}
	}
	return nil
}

func defaultDBPath() string {
	if v := os.Getenv("PULSE_DB"); v != "" {
		return v
	}
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "pulse", "pulse.db")
}
