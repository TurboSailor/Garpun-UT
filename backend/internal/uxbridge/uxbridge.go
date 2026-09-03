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
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"

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
//
// The bridge does not talk to the Waydroid container. Android notifications
// reach this phone through a dedicated relay (waydnotif.turbosailor), which posts
// them into the Lomiri shade; the freedesktop source below then observes them
// like any other notification. Polling the container here as well would post
// and forward everything twice.
type Options struct {
	EnableFreedesktop bool
	EnableCalls       bool
	EnableMusic       bool
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

	// fdCall is a plain session bus connection, used to close a notification
	// on the phone when the watch dismisses it. The monitor connection cannot
	// make calls.
	fdCall *dbus.Conn
	// fdPending holds Notify calls whose reply (and therefore the server's own
	// notification id) has not arrived yet, keyed by caller and serial.
	fdPending map[fdCallKey]int32
	fdPendOrd []fdCallKey
	// fdIDs and fdServer translate between the notification server's ids and
	// ours, which is what makes retraction and replacement possible.
	fdIDs    map[uint32]int32
	fdServer map[int32]uint32
	fdIDOrd  []uint32

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
	ctx, cancel := context.WithCancel(context.Background())
	b := &Bridge{
		log:       log,
		opts:      opts,
		out:       make(chan Notification, 256),
		ids:       map[string]int32{},
		nextID:    1,
		appName:   map[string]string{},
		fdPending: map[fdCallKey]int32{},
		fdIDs:     map[uint32]int32{},
		fdServer:  map[int32]uint32{},
		ctx:       ctx,
		cancel:    cancel,
	}

	if opts.EnableFreedesktop {
		if err := b.startFreedesktop(); err != nil {
			log.Warn("uxbridge: freedesktop notifications unavailable", "err", err)
		}
	}
	if opts.EnableCalls {
		if err := b.startCalls(); err != nil {
			log.Debug("uxbridge: telephony unavailable", "err", err)
		}
	}
	if opts.EnableMusic {
		if err := b.startMusic(); err != nil {
			log.Debug("uxbridge: media player unavailable", "err", err)
		}
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
		b.mu.Lock()
		callConn := b.fdCall
		b.fdCall = nil
		b.mu.Unlock()
		if callConn != nil {
			callConn.Close()
		}
		close(b.out)
	})
}

// -------------------------------------------------- freedesktop id mapping ---

// fdCallKey identifies one pending Notify call: the caller's unique bus name
// plus the call serial, which is what the reply refers back to.
type fdCallKey struct {
	caller string
	serial uint32
}

// fdPendingLimit caps the in-flight Notify calls the bridge remembers. Replies
// come back in milliseconds; anything older is a caller that never got one.
const fdPendingLimit = 64

func (b *Bridge) rememberNotifyCall(caller string, serial uint32, id int32) {
	key := fdCallKey{caller: caller, serial: serial}
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.fdPending[key]; !ok {
		b.fdPendOrd = append(b.fdPendOrd, key)
	}
	b.fdPending[key] = id
	for len(b.fdPendOrd) > fdPendingLimit {
		delete(b.fdPending, b.fdPendOrd[0])
		b.fdPendOrd = b.fdPendOrd[1:]
	}
}

// resolveNotifyCall records the server id a Notify call was answered with.
func (b *Bridge) resolveNotifyCall(caller string, serial, serverID uint32) {
	key := fdCallKey{caller: caller, serial: serial}
	b.mu.Lock()
	defer b.mu.Unlock()
	id, ok := b.fdPending[key]
	if !ok {
		return
	}
	delete(b.fdPending, key)
	for i, k := range b.fdPendOrd {
		if k == key {
			b.fdPendOrd = append(b.fdPendOrd[:i], b.fdPendOrd[i+1:]...)
			break
		}
	}
	if old, ok := b.fdServer[id]; ok && old != serverID {
		delete(b.fdIDs, old)
	}
	if _, ok := b.fdIDs[serverID]; !ok {
		b.fdIDOrd = append(b.fdIDOrd, serverID)
	}
	b.fdIDs[serverID] = id
	b.fdServer[id] = serverID
	for len(b.fdIDOrd) > historyLimit {
		stale := b.fdIDOrd[0]
		b.fdIDOrd = b.fdIDOrd[1:]
		if bridgeID, ok := b.fdIDs[stale]; ok {
			delete(b.fdIDs, stale)
			if b.fdServer[bridgeID] == stale {
				delete(b.fdServer, bridgeID)
			}
		}
	}
}

// bridgeIDForServer maps a server side notification id back to ours.
func (b *Bridge) bridgeIDForServer(serverID uint32) (int32, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id, ok := b.fdIDs[serverID]
	return id, ok
}

// retractServerNotification turns a card the shell closed into a retraction
// for the watch. Repeated calls for the same notification are harmless: the
// history entry is already marked removed and nothing is emitted twice.
func (b *Bridge) retractServerNotification(serverID uint32) {
	id, ok := b.bridgeIDForServer(serverID)
	if !ok {
		return
	}
	b.retract(id)
}

// retract emits the removal of a notification the bridge still holds live.
func (b *Bridge) retract(id int32) {
	n, live := b.find(id)
	if !live {
		return
	}
	b.emit(Notification{
		ID:       id,
		Source:   n.Source,
		AppID:    n.AppID,
		AppName:  n.AppName,
		Category: n.Category,
		Removed:  true,
	})
}

// DismissNotification is what the watch asks for when the user clears a card
// there: close it on the phone as well, then retract it so the watch list and
// the phone shade stay in step. The close is best effort — a card posted by
// another app may already be gone — but the retraction always happens, which
// is what stops notifications piling up on the wrist.
func (b *Bridge) DismissNotification(id int32) error {
	b.mu.Lock()
	conn := b.fdCall
	serverID, haveServer := b.fdServer[id]
	b.mu.Unlock()

	var err error
	if conn != nil && haveServer {
		obj := conn.Object(ifaceNotifications, "/org/freedesktop/Notifications")
		call := obj.Call(ifaceNotifications+".CloseNotification", 0, serverID)
		err = call.Err
	}
	b.retract(id)
	if err != nil {
		return fmt.Errorf("uxbridge: close notification %d: %w", serverID, err)
	}
	return nil
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
