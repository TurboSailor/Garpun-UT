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

const (
	ifaceNotifications = "org.freedesktop.Notifications"
	monitorRule        = "type='method_call',interface='org.freedesktop.Notifications',member='Notify'"
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

	call := conn.BusObject().Call("org.freedesktop.DBus.Monitoring.BecomeMonitor", 0, []string{monitorRule}, uint32(0))
	if call.Err != nil {
		b.log.Debug("uxbridge: BecomeMonitor rejected, falling back to eavesdrop", "err", call.Err)
		fallback := conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, monitorRule+",eavesdrop=true")
		if fallback.Err != nil {
			conn.Close()
			return fmt.Errorf("uxbridge: eavesdrop on notifications: %w", fallback.Err)
		}
	}

	msgs := make(chan *dbus.Message, 64)
	conn.Eavesdrop(msgs)

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

// fdSeq namespaces synthetic keys for freedesktop notifications. The server's
// own id is a return value we never see, so every Notify call gets a fresh id.
var fdSeq atomic.Uint64

// selfPackage is this application's click package. The shell addresses it by
// several names depending on the path a notification took: the bare package,
// package_hook, or package_hook_version.
const selfPackage = "cc.zachy.pulse"

// isSelfNotification reports whether an application identifier belongs to this
// app, in any of the forms the shell may present.
func isSelfNotification(id string) bool {
	if id == "" {
		return false
	}
	if id == "pulse" || id == selfPackage {
		return true
	}
	// package_hook and package_hook_version.
	return strings.HasPrefix(id, selfPackage+"_")
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
	if msg.Type != dbus.TypeMethodCall {
		return
	}
	if iface, _ := msg.Headers[dbus.FieldInterface].Value().(string); iface != ifaceNotifications {
		return
	}
	if member, _ := msg.Headers[dbus.FieldMember].Value().(string); member != "Notify" {
		return
	}
	n, ok := parseNotifyCall(msg.Body)
	if !ok {
		return
	}
	// Our own notifications must not loop back in. Anything relayed through
	// the push service is redelivered here attributed to this click package,
	// so it would reach the watch twice: once with the real Android app name
	// and once as "cc.zachy.pulse_pulse".
	if isSelfNotification(n.AppID) || isSelfNotification(n.AppName) {
		return
	}
	// Volume and brightness bars arrive here too; they are screen furniture,
	// not something to buzz a wrist for.
	if len(msg.Body) >= 7 {
		if hints, ok := msg.Body[6].(map[string]dbus.Variant); ok {
			if marker, overlay := isSystemOverlay(hints); overlay {
				b.log.Debug("uxbridge: ignoring system overlay",
					"app", n.AppName, "title", n.Title, "hint", marker)
				return
			}
		}
	}
	n.ID = b.idFor("fd:" + strconv.FormatUint(fdSeq.Add(1), 10))
	b.emit(n)
}

// parseNotifyCall decodes the Notify argument list:
// app_name s, replaces_id u, app_icon s, summary s, body s, actions as,
// hints a{sv}, expire_timeout i.
func parseNotifyCall(body []any) (Notification, bool) {
	if len(body) < 5 {
		return Notification{}, false
	}
	appName, _ := body[0].(string)
	summary, _ := body[3].(string)
	text, _ := body[4].(string)
	if summary == "" && text == "" {
		return Notification{}, false
	}

	appID := appName
	category := ""
	if len(body) >= 7 {
		if hints, ok := body[6].(map[string]dbus.Variant); ok {
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
	return n, true
}
