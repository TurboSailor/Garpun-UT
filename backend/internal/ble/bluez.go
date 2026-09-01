// Package ble is a minimal BlueZ (>=5.50) GATT central client built directly on
// the system D-Bus. It is written for Ubuntu Touch, where bluetoothd runs as
// root and the phablet user is a member of the "bluetooth" group.
package ble

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/godbus/dbus/v5"
)

const (
	busName = "org.bluez"

	ifaceAdapter        = "org.bluez.Adapter1"
	ifaceDevice         = "org.bluez.Device1"
	ifaceService        = "org.bluez.GattService1"
	ifaceChar           = "org.bluez.GattCharacteristic1"
	ifaceDesc           = "org.bluez.GattDescriptor1"
	ifaceAgentManager   = "org.bluez.AgentManager1"
	ifaceProps          = "org.freedesktop.DBus.Properties"
	ifaceObjectManager  = "org.freedesktop.DBus.ObjectManager"
	memberPropsChanged  = "org.freedesktop.DBus.Properties.PropertiesChanged"
	memberIfacesAdded   = "org.freedesktop.DBus.ObjectManager.InterfacesAdded"
	memberIfacesRemoved = "org.freedesktop.DBus.ObjectManager.InterfacesRemoved"
)

// ErrNotFound is returned when a requested BlueZ object does not exist.
var ErrNotFound = errors.New("ble: object not found")

type objectMap map[dbus.ObjectPath]map[string]map[string]dbus.Variant

// Bus is a shared connection to bluetoothd with a cached object tree and a
// fan-out signal dispatcher. All exported methods are safe for concurrent use.
type Bus struct {
	conn *dbus.Conn

	mu      sync.RWMutex
	objects objectMap

	subMu sync.Mutex
	subs  map[int]*subscription
	nextI int
}

type subscription struct {
	path   dbus.ObjectPath // empty matches any
	iface  string          // empty matches any
	fn     func(path dbus.ObjectPath, iface string, changed map[string]dbus.Variant)
	added  func(path dbus.ObjectPath, ifaces map[string]map[string]dbus.Variant)
	remove func(path dbus.ObjectPath, ifaces []string)
}

// Dial connects to the system bus and snapshots the BlueZ object tree.
func Dial() (*Bus, error) {
	conn, err := dbus.SystemBus()
	if err != nil {
		return nil, fmt.Errorf("ble: system bus: %w", err)
	}
	b := &Bus{conn: conn, objects: objectMap{}, subs: map[int]*subscription{}}

	for _, rule := range []string{
		"type='signal',sender='org.bluez',interface='org.freedesktop.DBus.Properties',member='PropertiesChanged'",
		"type='signal',sender='org.bluez',interface='org.freedesktop.DBus.ObjectManager',member='InterfacesAdded'",
		"type='signal',sender='org.bluez',interface='org.freedesktop.DBus.ObjectManager',member='InterfacesRemoved'",
	} {
		if call := conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, rule); call.Err != nil {
			return nil, fmt.Errorf("ble: add match: %w", call.Err)
		}
	}

	sig := make(chan *dbus.Signal, 256)
	conn.Signal(sig)
	go b.pump(sig)

	if err := b.refresh(); err != nil {
		return nil, err
	}
	return b, nil
}

// Conn exposes the underlying connection (used to export the pairing agent).
func (b *Bus) Conn() *dbus.Conn { return b.conn }

func (b *Bus) refresh() error {
	var objs objectMap
	err := b.conn.Object(busName, "/").Call(ifaceObjectManager+".GetManagedObjects", 0).Store(&objs)
	if err != nil {
		return fmt.Errorf("ble: GetManagedObjects: %w", err)
	}
	b.mu.Lock()
	b.objects = objs
	b.mu.Unlock()
	return nil
}

func (b *Bus) pump(sig <-chan *dbus.Signal) {
	for s := range sig {
		switch s.Name {
		case memberPropsChanged:
			if len(s.Body) < 2 {
				continue
			}
			iface, _ := s.Body[0].(string)
			changed, _ := s.Body[1].(map[string]dbus.Variant)
			invalidated, _ := s.Body[2].([]string)

			b.mu.Lock()
			if ifaces, ok := b.objects[s.Path]; ok {
				if props, ok := ifaces[iface]; ok {
					for k, v := range changed {
						props[k] = v
					}
					for _, k := range invalidated {
						delete(props, k)
					}
				}
			}
			b.mu.Unlock()

			b.dispatch(func(sub *subscription) {
				if sub.fn == nil || !sub.matches(s.Path, iface) {
					return
				}
				sub.fn(s.Path, iface, changed)
			})

		case memberIfacesAdded:
			if len(s.Body) < 2 {
				continue
			}
			path, _ := s.Body[0].(dbus.ObjectPath)
			ifaces, _ := s.Body[1].(map[string]map[string]dbus.Variant)

			b.mu.Lock()
			cur, ok := b.objects[path]
			if !ok {
				cur = map[string]map[string]dbus.Variant{}
				b.objects[path] = cur
			}
			for name, props := range ifaces {
				cur[name] = props
			}
			b.mu.Unlock()

			b.dispatch(func(sub *subscription) {
				if sub.added != nil {
					sub.added(path, ifaces)
				}
			})

		case memberIfacesRemoved:
			if len(s.Body) < 2 {
				continue
			}
			path, _ := s.Body[0].(dbus.ObjectPath)
			names, _ := s.Body[1].([]string)

			b.mu.Lock()
			if cur, ok := b.objects[path]; ok {
				for _, n := range names {
					delete(cur, n)
				}
				if len(cur) == 0 {
					delete(b.objects, path)
				}
			}
			b.mu.Unlock()

			b.dispatch(func(sub *subscription) {
				if sub.remove != nil {
					sub.remove(path, names)
				}
			})
		}
	}
}

func (s *subscription) matches(path dbus.ObjectPath, iface string) bool {
	if s.path != "" && s.path != path {
		return false
	}
	if s.iface != "" && s.iface != iface {
		return false
	}
	return true
}

func (b *Bus) dispatch(f func(*subscription)) {
	b.subMu.Lock()
	subs := make([]*subscription, 0, len(b.subs))
	for _, s := range b.subs {
		subs = append(subs, s)
	}
	b.subMu.Unlock()
	for _, s := range subs {
		f(s)
	}
}

func (b *Bus) subscribe(s *subscription) (cancel func()) {
	b.subMu.Lock()
	id := b.nextI
	b.nextI++
	b.subs[id] = s
	b.subMu.Unlock()
	return func() {
		b.subMu.Lock()
		delete(b.subs, id)
		b.subMu.Unlock()
	}
}

// prop reads a cached property, falling back to a live Get when absent.
func (b *Bus) prop(path dbus.ObjectPath, iface, name string) (dbus.Variant, bool) {
	b.mu.RLock()
	if ifaces, ok := b.objects[path]; ok {
		if props, ok := ifaces[iface]; ok {
			if v, ok := props[name]; ok {
				b.mu.RUnlock()
				return v, true
			}
		}
	}
	b.mu.RUnlock()

	var v dbus.Variant
	if err := b.conn.Object(busName, path).Call(ifaceProps+".Get", 0, iface, name).Store(&v); err != nil {
		return dbus.Variant{}, false
	}
	return v, true
}

func (b *Bus) setProp(path dbus.ObjectPath, iface, name string, value any) error {
	return b.conn.Object(busName, path).
		Call(ifaceProps+".Set", 0, iface, name, dbus.MakeVariant(value)).Err
}

// paths returns every cached object implementing iface, optionally restricted
// to descendants of parent.
func (b *Bus) paths(iface string, parent dbus.ObjectPath) []dbus.ObjectPath {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var out []dbus.ObjectPath
	for path, ifaces := range b.objects {
		if _, ok := ifaces[iface]; !ok {
			continue
		}
		if parent != "" && !strings.HasPrefix(string(path), string(parent)+"/") {
			continue
		}
		out = append(out, path)
	}
	return out
}

func strProp(v dbus.Variant, ok bool) string {
	if !ok {
		return ""
	}
	s, _ := v.Value().(string)
	return s
}

func boolProp(v dbus.Variant, ok bool) bool {
	if !ok {
		return false
	}
	b, _ := v.Value().(bool)
	return b
}

func int16Prop(v dbus.Variant, ok bool) int16 {
	if !ok {
		return 0
	}
	n, _ := v.Value().(int16)
	return n
}

func strsProp(v dbus.Variant, ok bool) []string {
	if !ok {
		return nil
	}
	s, _ := v.Value().([]string)
	return s
}
