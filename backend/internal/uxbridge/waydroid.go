package uxbridge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Android notifications live inside the Waydroid container and never reach the
// host session bus. The container does expose adbd on the bridge network
// though, and `dumpsys notification` prints the full posted list, so the
// bridge polls it and diffs successive snapshots.

const (
	adbTimeout     = 15 * time.Second
	dumpsysCommand = "dumpsys notification --noredact"
)

// shellRunner executes one command inside the Waydroid container.
type shellRunner interface {
	run(ctx context.Context, cmd string) ([]byte, error)
	name() string
	close()
}

// adbRunner talks to the container's adbd over TCP. The connection is kept
// open between polls and re-dialled after any failure.
type adbRunner struct {
	addr string

	mu   sync.Mutex
	conn *adbConn
}

func (r *adbRunner) name() string { return "adb " + r.addr }

func (r *adbRunner) run(ctx context.Context, cmd string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.conn == nil {
		c, err := adbDial(ctx, r.addr, adbTimeout)
		if err != nil {
			return nil, err
		}
		r.conn = c
	}
	out, err := r.conn.Shell(ctx, cmd, adbTimeout)
	if err != nil {
		r.conn.Close()
		r.conn = nil
		if errors.Is(err, errADBAuth) {
			return nil, err
		}
		return nil, fmt.Errorf("uxbridge: adb shell %q: %w", cmd, err)
	}
	return out, nil
}

func (r *adbRunner) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conn != nil {
		r.conn.Close()
		r.conn = nil
	}
}

// waydroidRunner shells into the container through the waydroid CLI, which
// needs root. Used only when adbd refuses us.
type waydroidRunner struct {
	exec func(context.Context, ...string) ([]byte, error)
}

func (r *waydroidRunner) name() string { return "waydroid shell" }

func (r *waydroidRunner) run(ctx context.Context, cmd string) ([]byte, error) {
	out, err := r.exec(ctx, "waydroid", "shell", "--", "sh", "-c", cmd)
	if err != nil {
		return nil, fmt.Errorf("uxbridge: waydroid shell %q: %w", cmd, err)
	}
	return out, nil
}

func (r *waydroidRunner) close() {}

// waydroidState is the previous snapshot of one notification, kept to detect
// text changes without re-sending unchanged records.
type waydroidState struct {
	title string
	body  string
	pkg   string
	tsMs  int64
	id    int32
}

func (b *Bridge) startWaydroid() {
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		b.pollWaydroid()
	}()
}

func (b *Bridge) pollWaydroid() {
	// The container is not required to be up when we start, and it may go away
	// and come back at any time: Waydroid is a session the user can stop. Keep
	// looking for it instead of disabling Android notifications for good.
	runner := b.waitForWaydroid()
	if runner == nil {
		return
	}
	defer func() {
		if runner != nil {
			runner.close()
		}
	}()
	b.log.Info("uxbridge: watching waydroid notifications", "via", runner.name())

	var (
		prev     map[string]waydroidState
		primed   bool
		resolved = map[string]bool{}
	)
	ticker := time.NewTicker(b.opts.WaydroidPollInterval)
	defer ticker.Stop()

	fails := 0
	for {
		out, err := runner.run(b.ctx, dumpsysCommand)
		if err != nil {
			if b.ctx.Err() != nil {
				return
			}
			// adbd can start demanding a key after a container restart; switch
			// transports rather than going blind.
			if errors.Is(err, errADBAuth) {
				if next := b.privilegedRunner(); next != nil {
					runner.close()
					runner = next
					fails = 0
					b.log.Warn("uxbridge: adbd wants authentication, switching to waydroid shell")
					continue
				}
			}
			b.log.Debug("uxbridge: waydroid poll failed", "err", err)

			// A stopped container fails every poll. Rather than spin against a
			// dead transport, drop it and wait for Waydroid to come back; the
			// baseline is re-primed so the returning notifications are not
			// replayed onto the watch.
			if fails++; fails >= waydroidFailsBeforeRedial {
				b.log.Info("uxbridge: waydroid stopped responding, waiting for it to come back")
				runner.close()
				runner = b.waitForWaydroid()
				if runner == nil {
					return
				}
				b.log.Info("uxbridge: waydroid is back", "via", runner.name())
				fails, primed, prev = 0, false, nil
				resolved = map[string]bool{}
			}
		} else {
			fails = 0
			cur := snapshotNotifications(parseDumpsysNotifications(string(out)))
			b.resolveAppNames(b.ctx, runner, unresolvedPackages(cur, resolved))
			if primed {
				b.diffWaydroid(prev, cur)
			} else {
				// The first poll only establishes a baseline: replaying every
				// notification already sitting in the shade would spam the watch.
				primed = true
				b.log.Debug("uxbridge: waydroid baseline", "count", len(cur))
			}
			prev = cur
		}

		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// waydroidFailsBeforeRedial is how many consecutive failed polls mean the
// container is gone rather than merely busy.
const waydroidFailsBeforeRedial = 3

// waitForWaydroid blocks until the container answers, backing off between
// attempts. It returns nil only when the bridge is shutting down.
func (b *Bridge) waitForWaydroid() shellRunner {
	const (
		firstDelay = 2 * time.Second
		maxDelay   = 30 * time.Second
	)
	delay := firstDelay
	for {
		if r := b.pickWaydroidRunner(); r != nil {
			return r
		}
		select {
		case <-b.ctx.Done():
			return nil
		case <-time.After(delay):
		}
		if delay < maxDelay {
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
		}
	}
}

// pickWaydroidRunner prefers adbd (no root needed) and falls back to the
// privileged waydroid CLI.
func (b *Bridge) pickWaydroidRunner() shellRunner {
	ctx, cancel := context.WithTimeout(b.ctx, adbTimeout)
	defer cancel()

	r := &adbRunner{addr: b.opts.WaydroidAddr}
	_, err := r.run(ctx, "true")
	if err == nil {
		return r
	}
	r.close()
	b.log.Debug("uxbridge: waydroid adbd unusable", "addr", b.opts.WaydroidAddr, "err", err)

	next := b.privilegedRunner()
	if next == nil {
		// Callers retry, so this must not shout on every attempt.
		b.log.Debug("uxbridge: waydroid unreachable and no privileged runner configured")
		return nil
	}
	if _, err := next.run(ctx, "true"); err != nil {
		b.log.Debug("uxbridge: waydroid unreachable", "err", err)
		return nil
	}
	return next
}

func (b *Bridge) privilegedRunner() shellRunner {
	if b.opts.PrivilegedRunner == nil {
		return nil
	}
	return &waydroidRunner{exec: b.opts.PrivilegedRunner}
}

func snapshotNotifications(records []dumpsysRecord) map[string]waydroidState {
	cur := make(map[string]waydroidState, len(records))
	for _, r := range records {
		if !r.userVisible() || r.Key == "" {
			continue
		}
		cur[r.Key] = waydroidState{
			title: r.Title,
			body:  r.Body(),
			pkg:   r.Pkg,
			tsMs:  r.When,
		}
	}
	return cur
}

func (b *Bridge) diffWaydroid(prev, cur map[string]waydroidState) {
	for key, st := range cur {
		old, existed := prev[key]
		if existed && old.title == st.title && old.body == st.body {
			continue
		}
		b.emit(Notification{
			ID:       b.idFor("wd:" + key),
			Source:   SourceWaydroid,
			AppID:    st.pkg,
			AppName:  b.AppName(st.pkg),
			Title:    st.title,
			Body:     st.body,
			Category: categoryFor(st.pkg, b.AppName(st.pkg)),
			TsMs:     st.tsMs,
		})
	}
	for key, st := range prev {
		if _, alive := cur[key]; alive {
			continue
		}
		id := b.idFor("wd:" + key)
		b.emit(Notification{
			ID:       id,
			Source:   SourceWaydroid,
			AppID:    st.pkg,
			AppName:  b.AppName(st.pkg),
			Title:    st.title,
			Body:     st.body,
			Category: categoryFor(st.pkg, b.AppName(st.pkg)),
			Removed:  true,
		})
		b.forgetKey("wd:" + key)
	}
}

// ------------------------------------------------------------- app labels ---

// unresolvedPackages lists the packages in a snapshot whose label has not been
// looked up yet, marking them so a failed lookup is not retried every poll.
func unresolvedPackages(cur map[string]waydroidState, seen map[string]bool) []string {
	var pkgs []string
	for _, st := range cur {
		if st.pkg == "" || seen[st.pkg] {
			continue
		}
		seen[st.pkg] = true
		pkgs = append(pkgs, st.pkg)
	}
	return pkgs
}

// resolveAppNames fills the label cache for packages we have not seen yet.
// Android does not expose localised labels over the shell, so this confirms the
// package exists and looks for a non-localised label; anything else falls back
// to the package's last segment.
func (b *Bridge) resolveAppNames(ctx context.Context, runner shellRunner, pkgs []string) {
	if len(pkgs) == 0 {
		return
	}
	installed := b.installedPackages(ctx, runner)
	for _, pkg := range pkgs {
		if installed != nil {
			if _, ok := installed[pkg]; !ok {
				continue
			}
		}
		out, err := runner.run(ctx, "dumpsys package "+pkg)
		if err != nil {
			continue
		}
		if label := parseNonLocalizedLabel(string(out)); label != "" {
			b.cacheAppName(pkg, label)
		}
	}
}

func (b *Bridge) installedPackages(ctx context.Context, runner shellRunner) map[string]struct{} {
	out, err := runner.run(ctx, "cmd package list packages")
	if err != nil {
		return nil
	}
	set := map[string]struct{}{}
	for _, ln := range strings.Split(string(out), "\n") {
		ln = strings.TrimSpace(ln)
		if name, ok := strings.CutPrefix(ln, "package:"); ok && name != "" {
			set[name] = struct{}{}
		}
	}
	return set
}

func parseNonLocalizedLabel(out string) string {
	for _, ln := range strings.Split(out, "\n") {
		idx := strings.Index(ln, "nonLocalizedLabel=")
		if idx < 0 {
			continue
		}
		v := ln[idx+len("nonLocalizedLabel="):]
		if end := strings.Index(v, " icon="); end >= 0 {
			v = v[:end]
		}
		v = strings.TrimSpace(v)
		if v != "" && v != "null" {
			return v
		}
	}
	return ""
}
