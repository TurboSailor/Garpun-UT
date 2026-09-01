package uxbridge

import (
	"fmt"
	"strconv"
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
	// Our own outgoing notifications would loop straight back in.
	if n.AppID == "pulse" || n.AppID == "cc.zachy.pulse" {
		return
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
