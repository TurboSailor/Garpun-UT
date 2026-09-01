// Package uxbridge collects everything the phone knows about the user's
// foreground experience — notifications, incoming calls and the active media
// player — and exposes it as one ordered stream plus a queryable history.
//
// The package deliberately knows nothing about watches. Sources push plain
// Notification values; garmin.go is the only file that translates them for a
// Garmin device, so a second consumer can be added without touching any source.
package uxbridge

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"pulse/backend/internal/garmin"
	"pulse/backend/internal/gfdi"
)

// historyLimit is how many notifications stay addressable by the watch. The
// watch only ever asks about recent ids, so a small ring is enough.
const historyLimit = 200

// Notification sources.
const (
	SourceFreedesktop = "freedesktop"
	SourceWaydroid    = "waydroid"
	SourceCall        = "call"
)

// Notification is one event in the bridge stream. A Removed notification
// repeats the id of the entry it retracts.
type Notification struct {
	ID       int32                       `json:"id"`
	Source   string                      `json:"source"`
	AppID    string                      `json:"appId"`
	AppName  string                      `json:"appName"`
	Title    string                      `json:"title"`
	Body     string                      `json:"body"`
	Category uint8                       `json:"category"`
	TsMs     int64                       `json:"tsMs"`
	Removed  bool                        `json:"removed"`
	Actions  []garmin.NotificationAction `json:"actions,omitempty"`
}

// Options configures which sources the bridge tries to start. Every source is
// optional: an unavailable one is logged once and skipped.
type Options struct {
	EnableFreedesktop    bool
	EnableWaydroid       bool
	WaydroidAddr         string
	WaydroidPollInterval time.Duration

	// PrivilegedRunner runs a command as root. It is used as the fallback path
	// into the Waydroid container when its adbd demands authentication. May be
	// nil, in which case that fallback is simply unavailable.
	PrivilegedRunner func(context.Context, ...string) ([]byte, error)
}

const (
	defaultWaydroidAddr = "192.168.240.112:5555"
	defaultPollInterval = 2 * time.Second
)

func (o *Options) applyDefaults() {
	if o.WaydroidAddr == "" {
		o.WaydroidAddr = defaultWaydroidAddr
	}
	if o.WaydroidPollInterval <= 0 {
		o.WaydroidPollInterval = defaultPollInterval
	}
}

// Bridge fans notification sources into a single channel. All exported methods
// are safe for concurrent use.
type Bridge struct {
	log  *slog.Logger
	opts Options

	out chan Notification

	mu      sync.Mutex
	ring    [historyLimit]Notification
	ringPos int
	ringLen int
	ids     map[string]int32 // stable source key -> notification id
	nextID  int32
	appName map[string]string

	calls *callSource
	music *musicSource

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	once   sync.Once
}

// New starts every enabled source. It only fails when nothing can be set up at
// all; individual source failures are logged and degrade the bridge quietly.
func New(log *slog.Logger, opts Options) (*Bridge, error) {
	if log == nil {
		log = slog.Default()
	}
	opts.applyDefaults()

	ctx, cancel := context.WithCancel(context.Background())
	b := &Bridge{
		log:     log,
		opts:    opts,
		out:     make(chan Notification, 256),
		ids:     map[string]int32{},
		nextID:  1,
		appName: map[string]string{},
		ctx:     ctx,
		cancel:  cancel,
	}

	if opts.EnableFreedesktop {
		if err := b.startFreedesktop(); err != nil {
			log.Warn("uxbridge: freedesktop notifications unavailable", "err", err)
		}
	}
	if opts.EnableWaydroid {
		b.startWaydroid()
	}
	if err := b.startCalls(); err != nil {
		log.Debug("uxbridge: telephony unavailable", "err", err)
	}
	if err := b.startMusic(); err != nil {
		log.Debug("uxbridge: media player unavailable", "err", err)
	}
	return b, nil
}

// Notifications is the event stream. Both new notifications and retractions
// arrive here; the consumer decides what to do with them.
func (b *Bridge) Notifications() <-chan Notification { return b.out }

// Close stops every source. The event channel is closed afterwards.
func (b *Bridge) Close() {
	b.once.Do(func() {
		b.cancel()
		b.wg.Wait()
		if b.calls != nil {
			b.calls.close()
		}
		if b.music != nil {
			b.music.close()
		}
		close(b.out)
	})
}

// ---------------------------------------------------------------- history ---

// idFor returns the stable id for a source key, minting one on first sight.
// Keys are opaque to the bridge; each source picks a namespace.
func (b *Bridge) idFor(key string) int32 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if id, ok := b.ids[key]; ok {
		return id
	}
	id := b.nextID
	b.nextID++
	b.ids[key] = id
	return id
}

// forgetKey drops the key->id mapping so a later notification with the same
// key gets a fresh id. The history entry stays addressable.
func (b *Bridge) forgetKey(key string) {
	b.mu.Lock()
	delete(b.ids, key)
	b.mu.Unlock()
}

func (b *Bridge) emit(n Notification) {
	if n.TsMs == 0 {
		n.TsMs = time.Now().UnixMilli()
	}
	if n.AppName == "" {
		n.AppName = b.AppName(n.AppID)
	}

	b.mu.Lock()
	b.ring[b.ringPos] = n
	b.ringPos = (b.ringPos + 1) % historyLimit
	if b.ringLen < historyLimit {
		b.ringLen++
	}
	b.mu.Unlock()

	select {
	case b.out <- n:
	default:
		b.log.Warn("uxbridge: event queue full, dropping notification", "id", n.ID, "source", n.Source)
	}
}

// Recent returns up to limit notifications, newest first.
func (b *Bridge) Recent(limit int) []Notification {
	b.mu.Lock()
	defer b.mu.Unlock()
	if limit <= 0 || limit > b.ringLen {
		limit = b.ringLen
	}
	out := make([]Notification, 0, limit)
	for i := range limit {
		idx := (b.ringPos - 1 - i + historyLimit*2) % historyLimit
		out = append(out, b.ring[idx])
	}
	return out
}

// find returns the newest live entry for id.
func (b *Bridge) find(id int32) (Notification, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := range b.ringLen {
		idx := (b.ringPos - 1 - i + historyLimit*2) % historyLimit
		if b.ring[idx].ID == id {
			return b.ring[idx], !b.ring[idx].Removed
		}
	}
	return Notification{}, false
}

// AppName resolves a human readable name for an application id, falling back
// to the last package segment.
func (b *Bridge) AppName(appID string) string {
	if appID == "" {
		return ""
	}
	b.mu.Lock()
	name, ok := b.appName[appID]
	b.mu.Unlock()
	if ok && name != "" {
		return name
	}
	return prettyAppID(appID)
}

func (b *Bridge) cacheAppName(appID, name string) {
	if appID == "" || name == "" {
		return
	}
	b.mu.Lock()
	b.appName[appID] = name
	b.mu.Unlock()
}

// prettyAppID turns "org.telegram.messenger" into "Messenger" — the best guess
// available without reading the app's resources.
func prettyAppID(appID string) string {
	seg := appID
	if i := strings.LastIndexByte(seg, '.'); i >= 0 && i+1 < len(seg) {
		seg = seg[i+1:]
	}
	if seg == "" {
		return appID
	}
	return strings.ToUpper(seg[:1]) + seg[1:]
}

// --------------------------------------------------------------- category ---

// categoryFor maps an application to a watch notification category. Matching
// runs against the package id and the display name together, most specific
// rule first.
func categoryFor(appID, appName string) uint8 {
	h := strings.ToLower(appID + " " + appName)
	switch {
	case strings.Contains(h, "sms"), strings.Contains(h, "messag"):
		return gfdi.CategorySMS
	case strings.Contains(h, "mail"):
		return gfdi.CategoryEmail
	case strings.Contains(h, "call"):
		return gfdi.CategoryIncomingCall
	case strings.Contains(h, "telegram"), strings.Contains(h, "whatsapp"),
		strings.Contains(h, "signal"), strings.Contains(h, "viber"):
		return gfdi.CategorySocial
	case strings.Contains(h, "calendar"):
		return gfdi.CategorySchedule
	default:
		return gfdi.CategoryOther
	}
}
