// Package fdnotify posts notifications to the desktop notification server.
//
// On Ubuntu Touch that server is Lomiri itself: the shell owns
// org.freedesktop.Notifications, so a Notify call is what puts an entry in the
// notification shade. This package is the write side of the same interface
// uxbridge only listens on.
package fdnotify

import (
	"fmt"
	"sync"

	"github.com/godbus/dbus/v5"
)

const (
	busName    = "org.freedesktop.Notifications"
	objectPath = "/org/freedesktop/Notifications"
	iface      = "org.freedesktop.Notifications"
)

// SourceHint marks a notification this project injected, and names where it
// originally came from. Consumers use it to tell an Android notification
// relayed into the shade from one the phone raised natively.
const SourceHint = "x-pulse-source"

// AppNameHint carries the human readable name of the originating application,
// which is not always what the shell shows as app_name.
const AppNameHint = "x-pulse-appname"

// Notification is one entry to place in the shade.
type Notification struct {
	// AppName is shown as the origin. Pass the Android application label so the
	// shade and the watch agree on who sent it.
	AppName string
	// AppID goes out as the desktop-entry hint; consumers read it as the
	// package identifier.
	AppID string
	// Source is the value of the x-pulse-source hint, e.g. "waydroid".
	Source  string
	Icon    string
	Summary string
	Body    string
	// ReplacesID updates an existing entry in place when non-zero.
	ReplacesID uint32
	// TimeoutMs follows the spec: -1 server default, 0 never expire.
	TimeoutMs int32
}

// Client is a connection to the notification server. It is safe for concurrent
// use.
type Client struct {
	mu   sync.Mutex
	conn *dbus.Conn
	obj  dbus.BusObject
}

// New dials the session bus and verifies the notification server is present,
// so a missing shell surfaces immediately instead of on the first post.
func New() (*Client, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("fdnotify: session bus: %w", err)
	}
	c := &Client{conn: conn, obj: conn.Object(busName, objectPath)}

	var has bool
	if err := conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, busName).Store(&has); err != nil {
		return nil, fmt.Errorf("fdnotify: probe %s: %w", busName, err)
	}
	if !has {
		return nil, fmt.Errorf("fdnotify: no owner for %s", busName)
	}
	return c, nil
}

// ServerInfo reports the notification server implementation, which is handy
// for logging exactly which shell is going to render our entries.
func (c *Client) ServerInfo() (name, vendor, version string, err error) {
	err = c.obj.Call(iface+".GetServerInformation", 0).Store(&name, &vendor, &version, new(string))
	if err != nil {
		return "", "", "", fmt.Errorf("fdnotify: server information: %w", err)
	}
	return name, vendor, version, nil
}

// Post places a notification in the shade and returns the server id, which is
// what CloseNotification and ReplacesID refer to later.
func (c *Client) Post(n Notification) (uint32, error) {
	hints := map[string]dbus.Variant{}
	if n.AppID != "" {
		hints["desktop-entry"] = dbus.MakeVariant(n.AppID)
	}
	if n.Source != "" {
		hints[SourceHint] = dbus.MakeVariant(n.Source)
	}
	if n.AppName != "" {
		hints[AppNameHint] = dbus.MakeVariant(n.AppName)
	}

	timeout := n.TimeoutMs
	if timeout == 0 {
		timeout = -1
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	var id uint32
	err := c.obj.Call(iface+".Notify", 0,
		n.AppName, n.ReplacesID, n.Icon, n.Summary, n.Body,
		[]string{}, hints, timeout,
	).Store(&id)
	if err != nil {
		return 0, fmt.Errorf("fdnotify: notify: %w", err)
	}
	return id, nil
}

// Close retracts a notification the server still shows.
func (c *Client) Close(id uint32) error {
	if id == 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.obj.Call(iface+".CloseNotification", 0, id).Err; err != nil {
		return fmt.Errorf("fdnotify: close %d: %w", id, err)
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
