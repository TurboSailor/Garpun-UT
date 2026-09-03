package uxbridge

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/godbus/dbus/v5"
)

// Native Lomiri notifications are only observable by eavesdropping: the
// notification server owns org.freedesktop.Notifications and nothing
// re-broadcasts what it receives. BecomeMonitor is the supported way to do
// that on dbus-daemon >= 1.9; older daemons need the AddMatch/eavesdrop hack.
//
// A monitor connection is one-way — the daemon refuses every method call on it
// — so this runs on its own private connection.

const ifaceNotifications = "org.freedesktop.Notifications"

// monitorRules is what the bridge needs to see. Notify alone is not enough:
// a card the user swipes away has to be retracted from the watch too, and the
// only handle the shell and the watch share is the server-side notification
// id, which appears exclusively in the reply to Notify. The bus cannot filter
// replies by interface, so every method_return is delivered and matched by
// reply serial in onFreedesktopMessage.
var monitorRules = []string{
	"type='method_call',interface='org.freedesktop.Notifications',member='Notify'",
	"type='method_call',interface='org.freedesktop.Notifications',member='CloseNotification'",
	"type='signal',interface='org.freedesktop.Notifications',member='NotificationClosed'",
	"type='method_return'",
}

// Reasons from the NotificationClosed signal. Expiry is not a dismissal: on
// Lomiri a Notify only ever produces a transient popup, which the server
// expires on its own while the entry in the shade lives on.
const (
	closeReasonExpired   = 1
	closeReasonDismissed = 2
	closeReasonClosed    = 3
)

func (b *Bridge) startFreedesktop() error {
	conn, err := dbus.SessionBusPrivate()
	if err != nil {
		return fmt.Errorf("uxbridge: session bus: %w", err)
	}
	if err := conn.Auth(nil); err != nil {
		conn.Close()
		return fmt.Errorf("uxbridge: session bus auth: %w", err)
	}
	if err := conn.Hello(); err != nil {
		conn.Close()
		return fmt.Errorf("uxbridge: session bus hello: %w", err)
	}

	call := conn.BusObject().Call("org.freedesktop.DBus.Monitoring.BecomeMonitor", 0, monitorRules, uint32(0))
	if call.Err != nil {
		b.log.Debug("uxbridge: BecomeMonitor rejected, falling back to eavesdrop", "err", call.Err)
		for _, rule := range monitorRules {
			fallback := conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, rule+",eavesdrop=true")
			if fallback.Err != nil {
				conn.Close()
				return fmt.Errorf("uxbridge: eavesdrop on notifications: %w", fallback.Err)
			}
		}
	}

	msgs := make(chan *dbus.Message, 64)
	conn.Eavesdrop(msgs)

	// A monitor cannot make calls, so closing a card on the phone (what the
	// watch asks for when the user dismisses it there) needs a second, plain
	// connection.
	if callConn, err := plainSessionBus(); err != nil {
		b.log.Debug("uxbridge: no session bus for closing notifications", "err", err)
	} else {
		b.mu.Lock()
		b.fdCall = callConn
		b.mu.Unlock()
	}

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		defer conn.Close()
		for {
			select {
			case <-b.ctx.Done():
				return
			case msg, ok := <-msgs:
				if !ok {
					return
				}
				b.onFreedesktopMessage(msg)
			}
		}
	}()
	b.log.Info("uxbridge: watching freedesktop notifications")
	return nil
}

func plainSessionBus() (*dbus.Conn, error) {
	conn, err := dbus.SessionBusPrivate()
	if err != nil {
		return nil, err
	}
	if err := conn.Auth(nil); err != nil {
		conn.Close()
		return nil, err
	}
	if err := conn.Hello(); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// fdSeq namespaces synthetic keys for freedesktop notifications.
var fdSeq atomic.Uint64

// selfPackage is this application's click package. The shell addresses it by
// several names depending on the path a notification took: the bare package,
// package_hook, or package_hook_version.
const selfPackage = "pulse.turbosailor"

// relayPackage is the Waydroid notification relay (a separate app). It posts
// Android notifications into the shade through the push service, so they reach
// this monitor attributed to that click package with the Android application
// name as the card summary. Recognising it is what keeps Android notifications
// tagged as such on the watch instead of arriving as "waydnotif.turbosailor".
const relayPackage = "waydnotif.turbosailor"

// isSelfNotification reports whether an application identifier belongs to this
// app, in any of the forms the shell may present.
func isSelfNotification(id string) bool {
	return matchesPackage(id, selfPackage) || id == "pulse"
}

// isRelayNotification reports whether the identifier belongs to the Waydroid
// relay.
func isRelayNotification(id string) bool {
	return matchesPackage(id, relayPackage)
}

// matchesPackage accepts the bare package plus the package_hook and
// package_hook_version forms the shell uses.
func matchesPackage(id, pkg string) bool {
	if id == "" {
		return false
	}
	return id == pkg || strings.HasPrefix(id, pkg+"_")
}

// Lomiri renders two very different things through this one interface: entries
// that land in the notification list, and transient on-screen overlays such as
// the volume and brightness bars. Only the first kind belongs on a watch.
//
// The overlays are marked by hints inherited from Unity. The "synchronous"
// ones replace each other in place instead of stacking, which is exactly what
// a volume bar does; "icon-only" and "transient" say outright that there is
// nothing worth keeping.
var osdHints = []string{
	"x-lomiri-private-synchronous",
	"x-canonical-private-synchronous",
	"x-lomiri-private-icon-only",
	"x-canonical-private-icon-only",
	"x-lomiri-non-shaped-icon",
	"x-canonical-non-shaped-icon",
}

// isSystemOverlay reports whether these hints describe an on-screen overlay
// rather than a real notification. It also names the marker, for logging.
func isSystemOverlay(hints map[string]dbus.Variant) (string, bool) {
	for _, h := range osdHints {
		if _, ok := hints[h]; ok {
			return h, true
		}
	}
	if v, ok := hints["transient"]; ok && truthy(v) {
		return "transient", true
	}
	// A bare progress reading with no text of its own is a bar, not a message.
	if _, ok := hints["value"]; ok {
		return "value", true
	}
	return "", false
}

// truthy reads a hint that may arrive as a bool or as a number.
func truthy(v dbus.Variant) bool {
	switch x := v.Value().(type) {
	case bool:
		return x
	case uint8:
		return x != 0
	case int32:
		return x != 0
	case uint32:
		return x != 0
	}
	return false
}

func (b *Bridge) onFreedesktopMessage(msg *dbus.Message) {
	iface, _ := msg.Headers[dbus.FieldInterface].Value().(string)
	member, _ := msg.Headers[dbus.FieldMember].Value().(string)

	switch {
	case msg.Type == dbus.TypeMethodReply:
		b.onNotifyReply(msg)
	case iface != ifaceNotifications:
		return
	case msg.Type == dbus.TypeSignal && member == "NotificationClosed":
		b.onNotificationClosed(msg.Body)
	case msg.Type == dbus.TypeMethodCall && member == "CloseNotification":
		if len(msg.Body) > 0 {
			if id, ok := msg.Body[0].(uint32); ok {
				b.retractServerNotification(id)
			}
		}
	case msg.Type == dbus.TypeMethodCall && member == "Notify":
		b.onNotifyCall(msg)
	}
}

func (b *Bridge) onNotifyCall(msg *dbus.Message) {
	n, replaces, overlay, ok := parseNotifyCall(msg.Body)
	if !ok {
		return
	}
	// Our own notifications must not loop back in. Anything relayed through
	// the push service is redelivered here attributed to this click package,
	// so it would reach the watch twice: once with the real Android app name
	// and once as "pulse.turbosailor_pulse".
	if isSelfNotification(n.AppID) || isSelfNotification(n.AppName) {
		return
	}
	// Volume and brightness bars arrive here too; they are screen furniture,
	// not something to buzz a wrist for.
	if overlay != "" {
		b.log.Debug("uxbridge: ignoring system overlay",
			"app", n.AppName, "title", n.Title, "hint", overlay)
		return
	}

	// A card that replaces another one (message counters, download progress)
	// keeps the same id, so the watch updates the entry it already holds
	// instead of stacking a new one that nobody ever clears.
	if replaces != 0 {
		if id, ok := b.bridgeIDForServer(replaces); ok {
			n.ID = id
		}
	}
	if n.ID == 0 {
		n.ID = b.idFor("fd:" + strconv.FormatUint(fdSeq.Add(1), 10))
	}

	sender, _ := msg.Headers[dbus.FieldSender].Value().(string)
	b.rememberNotifyCall(sender, msg.Serial(), n.ID)
	b.emit(n)
}

// onNotifyReply pairs the server's id with ours. Every method reply on the bus
// lands here, so anything that does not match a Notify call we saw is ignored.
func (b *Bridge) onNotifyReply(msg *dbus.Message) {
	if len(msg.Body) == 0 {
		return
	}
	serverID, ok := msg.Body[0].(uint32)
	if !ok || serverID == 0 {
		return
	}
	dest, _ := msg.Headers[dbus.FieldDestination].Value().(string)
	serial, _ := msg.Headers[dbus.FieldReplySerial].Value().(uint32)
	b.resolveNotifyCall(dest, serial, serverID)
}

func (b *Bridge) onNotificationClosed(body []any) {
	if len(body) < 2 {
		return
	}
	serverID, ok := body[0].(uint32)
	if !ok {
		return
	}
	reason, _ := body[1].(uint32)
	switch reason {
	case closeReasonDismissed, closeReasonClosed:
		b.retractServerNotification(serverID)
	case closeReasonExpired:
		// The popup timed out; the entry itself is still in the shade.
	}
}

// parseNotifyCall decodes the Notify argument list:
// app_name s, replaces_id u, app_icon s, summary s, body s, actions as,
// hints a{sv}, expire_timeout i.
// It returns the replaces_id the caller wants to update, the hint that marks
// this call as a system overlay (empty for a real notification), and whether
// the call carried anything worth showing.
func parseNotifyCall(body []any) (Notification, uint32, string, bool) {
	if len(body) < 5 {
		return Notification{}, 0, "", false
	}
	appName, _ := body[0].(string)
	replaces, _ := body[1].(uint32)
	summary, _ := body[3].(string)
	text, _ := body[4].(string)
	if summary == "" && text == "" {
		return Notification{}, 0, "", false
	}
	appID := appName
	category := ""
	overlay := ""
	if len(body) >= 7 {
		if hints, ok := body[6].(map[string]dbus.Variant); ok {
			if marker, isOverlay := isSystemOverlay(hints); isOverlay {
				overlay = marker
			}
			if v, ok := hints["desktop-entry"]; ok {
				if s, ok := v.Value().(string); ok && s != "" {
					appID = s
				}
			}
			if v, ok := hints["category"]; ok {
				category, _ = v.Value().(string)
			}
		}
	}

	n := Notification{
		Source:   SourceFreedesktop,
		AppID:    appID,
		AppName:  appName,
		Title:    summary,
		Body:     text,
		Category: categoryFor(appID, appName+" "+category),
	}
	// A card posted by the Waydroid relay carries the Android application name
	// as its summary and the message as its body. Left alone, the watch would
	// show the relay's package as the application and its summary as the title.
	// The Android package itself is not in the card, so the app name is all
	// there is to go on.
	if isRelayNotification(appID) || isRelayNotification(appName) {
		n.Source = SourceWaydroid
		n.AppName = summary
		n.Title = text
		n.Body = ""
		n.Category = categoryFor(summary, summary+" "+category)
	}
	return n, replaces, overlay, true
}
