package uxbridge

import (
	"fmt"
	"strings"
	"sync"

	"github.com/godbus/dbus/v5"

	"pulse/backend/internal/garmin"
	"pulse/backend/internal/gfdi"
)

// Ubuntu Touch drives telephony through ofono on the system bus. ofono predates
// the standard properties interface, so it emits its own PropertyChanged
// signals with a (string, variant) payload.

const (
	ifaceOfonoManager  = "org.ofono.Manager"
	ifaceOfonoCallMgr  = "org.ofono.VoiceCallManager"
	ifaceOfonoCall     = "org.ofono.VoiceCall"
	ofonoBus           = "org.ofono"
	memberPropChanged  = "org.ofono.VoiceCall.PropertyChanged"
	memberCallAdded    = "org.ofono.VoiceCallManager.CallAdded"
	memberCallRemoved  = "org.ofono.VoiceCallManager.CallRemoved"
	callKeyPrefix      = "call:"
	callActionAcceptLb = "Answer"
	callActionRejectLb = "Reject"
)

type callSource struct {
	conn *dbus.Conn

	mu      sync.Mutex
	active  dbus.ObjectPath
	ringing bool
}

func (c *callSource) close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

func (b *Bridge) startCalls() error {
	conn, err := dbus.SystemBusPrivate()
	if err != nil {
		return fmt.Errorf("uxbridge: system bus: %w", err)
	}
	if err := conn.Auth(nil); err != nil {
		conn.Close()
		return fmt.Errorf("uxbridge: system bus auth: %w", err)
	}
	if err := conn.Hello(); err != nil {
		conn.Close()
		return fmt.Errorf("uxbridge: system bus hello: %w", err)
	}

	var modems []struct {
		Path  dbus.ObjectPath
		Props map[string]dbus.Variant
	}
	if err := conn.Object(ofonoBus, "/").Call(ifaceOfonoManager+".GetModems", 0).Store(&modems); err != nil {
		conn.Close()
		return fmt.Errorf("uxbridge: ofono GetModems: %w", err)
	}

	for _, rule := range []string{
		"type='signal',sender='org.ofono',interface='org.ofono.VoiceCall',member='PropertyChanged'",
		"type='signal',sender='org.ofono',interface='org.ofono.VoiceCallManager',member='CallAdded'",
		"type='signal',sender='org.ofono',interface='org.ofono.VoiceCallManager',member='CallRemoved'",
	} {
		if call := conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, rule); call.Err != nil {
			conn.Close()
			return fmt.Errorf("uxbridge: ofono add match: %w", call.Err)
		}
	}

	src := &callSource{conn: conn}
	b.calls = src

	sig := make(chan *dbus.Signal, 32)
	conn.Signal(sig)

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		for {
			select {
			case <-b.ctx.Done():
				return
			case s, ok := <-sig:
				if !ok {
					return
				}
				b.onOfonoSignal(s)
			}
		}
	}()

	// Pick up a call that is already ringing when the daemon starts.
	for _, m := range modems {
		var calls []struct {
			Path  dbus.ObjectPath
			Props map[string]dbus.Variant
		}
		if err := conn.Object(ofonoBus, m.Path).Call(ifaceOfonoCallMgr+".GetCalls", 0).Store(&calls); err != nil {
			continue
		}
		for _, c := range calls {
			b.onCallState(c.Path, ofonoStr(c.Props, "State"), ofonoStr(c.Props, "LineIdentification"), ofonoStr(c.Props, "Name"))
		}
	}
	b.log.Info("uxbridge: watching ofono voice calls", "modems", len(modems))
	return nil
}

func ofonoStr(props map[string]dbus.Variant, key string) string {
	if v, ok := props[key]; ok {
		s, _ := v.Value().(string)
		return s
	}
	return ""
}

func (b *Bridge) onOfonoSignal(s *dbus.Signal) {
	switch s.Name {
	case memberCallAdded:
		if len(s.Body) < 2 {
			return
		}
		path, _ := s.Body[0].(dbus.ObjectPath)
		props, _ := s.Body[1].(map[string]dbus.Variant)
		b.onCallState(path, ofonoStr(props, "State"), ofonoStr(props, "LineIdentification"), ofonoStr(props, "Name"))
	case memberCallRemoved:
		if len(s.Body) < 1 {
			return
		}
		path, _ := s.Body[0].(dbus.ObjectPath)
		b.onCallState(path, "disconnected", "", "")
	case memberPropChanged:
		if len(s.Body) < 2 {
			return
		}
		name, _ := s.Body[0].(string)
		if name != "State" {
			return
		}
		v, _ := s.Body[1].(dbus.Variant)
		state, _ := v.Value().(string)
		b.onCallState(s.Path, state, b.callLine(s.Path), "")
	}
}

// callLine reads the caller id lazily; the PropertyChanged signal only carries
// the property that changed.
func (b *Bridge) callLine(path dbus.ObjectPath) string {
	if b.calls == nil || b.calls.conn == nil {
		return ""
	}
	var props map[string]dbus.Variant
	if err := b.calls.conn.Object(ofonoBus, path).Call(ifaceOfonoCall+".GetProperties", 0).Store(&props); err != nil {
		return ""
	}
	if line := ofonoStr(props, "LineIdentification"); line != "" {
		return line
	}
	return ofonoStr(props, "Name")
}

func (b *Bridge) onCallState(path dbus.ObjectPath, state, line, name string) {
	if path == "" {
		return
	}
	key := callKeyPrefix + string(path)
	title := name
	if title == "" {
		title = line
	}
	if title == "" {
		title = "Unknown caller"
	}

	switch state {
	case "incoming", "waiting":
		b.calls.mu.Lock()
		b.calls.active, b.calls.ringing = path, true
		b.calls.mu.Unlock()
		b.emit(Notification{
			ID:       b.idFor(key),
			Source:   SourceCall,
			AppID:    "telephony",
			AppName:  "Phone",
			Title:    title,
			Body:     line,
			Category: gfdi.CategoryIncomingCall,
			Actions: []garmin.NotificationAction{
				{Code: gfdi.ActionAcceptIncomingCall, Icon: 1, Label: callActionAcceptLb},
				{Code: gfdi.ActionRejectIncomingCall, Icon: 2, Label: callActionRejectLb},
			},
		})
	case "active":
		b.calls.mu.Lock()
		b.calls.active, b.calls.ringing = path, false
		b.calls.mu.Unlock()
	case "disconnected":
		b.calls.mu.Lock()
		if b.calls.active == path {
			b.calls.active, b.calls.ringing = "", false
		}
		b.calls.mu.Unlock()
		b.emit(Notification{
			ID:       b.idFor(key),
			Source:   SourceCall,
			AppID:    "telephony",
			AppName:  "Phone",
			Title:    title,
			Category: gfdi.CategoryIncomingCall,
			Removed:  true,
		})
		b.forgetKey(key)
	}
}

// AcceptCall answers the call currently ringing.
func (b *Bridge) AcceptCall() error { return b.callAction("Answer", true) }

// RejectCall hangs up the call currently ringing or in progress.
func (b *Bridge) RejectCall() error { return b.callAction("Hangup", false) }

func (b *Bridge) callAction(method string, needRinging bool) error {
	if b.calls == nil || b.calls.conn == nil {
		return fmt.Errorf("uxbridge: telephony unavailable")
	}
	b.calls.mu.Lock()
	path, ringing := b.calls.active, b.calls.ringing
	b.calls.mu.Unlock()
	if path == "" {
		return fmt.Errorf("uxbridge: no active call")
	}
	if needRinging && !ringing {
		return fmt.Errorf("uxbridge: call is not ringing")
	}
	call := b.calls.conn.Object(ofonoBus, path).Call(ifaceOfonoCall+"."+method, 0)
	if call.Err != nil {
		return fmt.Errorf("uxbridge: ofono %s: %w", strings.ToLower(method), call.Err)
	}
	return nil
}
