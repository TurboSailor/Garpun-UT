// Package postal posts notifications through the Ubuntu Touch push service.
//
// Two mechanisms can put something on this phone's screen and they are not
// equivalent. org.freedesktop.Notifications gives a transient bubble that
// Lomiri draws and forgets. com.lomiri.Postal is the native path: entries
// persist in the notification list, survive the screen going off and are what
// the user actually finds after the fact.
//
// Postal will not deliver for an application that does not declare a
// push-helper hook in its click manifest — it launches that helper to turn the
// posted payload into the final notification. Ours is a pass-through, so what
// Post sends here is exactly what the shell renders.
package postal

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/godbus/dbus/v5"
)

const (
	busName = "com.lomiri.Postal"
	iface   = "com.lomiri.Postal"
)

// Card is the visible part of a notification.
type Card struct {
	Summary string `json:"summary"`
	Body    string `json:"body,omitempty"`
	// Popup shows a bubble on arrival; Persist keeps it in the list afterwards.
	Popup   bool     `json:"popup"`
	Persist bool     `json:"persist"`
	Actions []string `json:"actions,omitempty"`
	Icon    string   `json:"icon,omitempty"`
}

// Message is the envelope the push helper passes straight through.
type Message struct {
	Notification struct {
		Tag  string `json:"tag,omitempty"`
		Card *Card  `json:"card,omitempty"`
		// EmblemCounter drives the badge on the app icon.
		EmblemCounter *EmblemCounter `json:"emblem-counter,omitempty"`
		Vibrate       bool           `json:"vibrate,omitempty"`
		Sound         bool           `json:"sound,omitempty"`
	} `json:"notification"`
}

// EmblemCounter is the numeric badge shown over the application icon.
type EmblemCounter struct {
	Count   int  `json:"count"`
	Visible bool `json:"visible"`
}

// Client talks to the push service for one application id.
type Client struct {
	appID string
	path  dbus.ObjectPath

	mu   sync.Mutex
	conn *dbus.Conn
	obj  dbus.BusObject
}

// New connects for the given click application id, e.g. "cc.zachy.pulse_pulse".
// The package part of that id selects the object path the service listens on.
func New(appID string) (*Client, error) {
	if appID == "" {
		return nil, fmt.Errorf("postal: empty application id")
	}
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("postal: session bus: %w", err)
	}

	path := objectPath(appID)
	c := &Client{
		appID: appID,
		path:  path,
		conn:  conn,
		obj:   conn.Object(busName, path),
	}

	var has bool
	if err := conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, busName).Store(&has); err != nil {
		return nil, fmt.Errorf("postal: probe %s: %w", busName, err)
	}
	if !has {
		return nil, fmt.Errorf("postal: no owner for %s", busName)
	}
	return c, nil
}

// objectPath maps an application id to the path the push service exports.
// The service keys on the click package, so "cc.zachy.pulse_pulse" becomes
// /com/lomiri/Postal/cc_2ezachy_2epulse: every byte outside [A-Za-z0-9] is
// escaped as _<hex>.
func objectPath(appID string) dbus.ObjectPath {
	pkg := appID
	if i := strings.Index(pkg, "_"); i >= 0 {
		pkg = pkg[:i]
	}
	var b strings.Builder
	b.WriteString("/com/lomiri/Postal/")
	for i := range len(pkg) {
		ch := pkg[i]
		switch {
		case ch >= 'a' && ch <= 'z', ch >= 'A' && ch <= 'Z', ch >= '0' && ch <= '9':
			b.WriteByte(ch)
		default:
			fmt.Fprintf(&b, "_%02x", ch)
		}
	}
	return dbus.ObjectPath(b.String())
}

// Path reports the object path in use, for logging.
func (c *Client) Path() string { return string(c.path) }

// Post delivers one notification. Tag identifies it for later removal.
func (c *Client) Post(tag string, card Card) error {
	var m Message
	m.Notification.Tag = tag
	m.Notification.Card = &card

	payload, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("postal: encode: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.obj.Call(iface+".Post", 0, c.appID, string(payload)).Err; err != nil {
		return fmt.Errorf("postal: post: %w", err)
	}
	return nil
}

// ListPersistent returns the tags still held by the notification list.
func (c *Client) ListPersistent() ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var tags []string
	if err := c.obj.Call(iface+".ListPersistent", 0, c.appID).Store(&tags); err != nil {
		return nil, fmt.Errorf("postal: list persistent: %w", err)
	}
	return tags, nil
}

// ClearPersistent removes the named tags from the notification list and
// reports how many entries went away.
//
// The service takes one tag per call — passing an array, or several tags in
// one call, is rejected as invalid arguments — so this loops.
func (c *Client) ClearPersistent(tags ...string) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cleared := 0
	for _, tag := range tags {
		if tag == "" {
			continue
		}
		var n uint32
		if err := c.obj.Call(iface+".ClearPersistent", 0, c.appID, tag).Store(&n); err != nil {
			return cleared, fmt.Errorf("postal: clear persistent %q: %w", tag, err)
		}
		cleared += int(n)
	}
	return cleared, nil
}

// SetCounter updates the badge on the application icon.
func (c *Client) SetCounter(count int, visible bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.obj.Call(iface+".SetCounter", 0, c.appID, int32(count), visible).Err; err != nil {
		return fmt.Errorf("postal: set counter: %w", err)
	}
	return nil
}

// Disconnect drops the bus connection.
func (c *Client) Disconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
}
