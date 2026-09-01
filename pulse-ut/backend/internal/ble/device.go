package ble

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

// Device wraps org.bluez.Device1.
type Device struct {
	bus     *Bus
	adapter *Adapter
	path    dbus.ObjectPath
	addr    string
}

func (d *Device) Path() dbus.ObjectPath { return d.path }
func (d *Device) Address() string       { return d.addr }

func (d *Device) Name() string {
	if n := strProp(d.bus.prop(d.path, ifaceDevice, "Name")); n != "" {
		return n
	}
	return strProp(d.bus.prop(d.path, ifaceDevice, "Alias"))
}

func (d *Device) Connected() bool {
	return boolProp(d.bus.prop(d.path, ifaceDevice, "Connected"))
}

func (d *Device) Paired() bool {
	return boolProp(d.bus.prop(d.path, ifaceDevice, "Paired"))
}

func (d *Device) obj() dbus.BusObject { return d.bus.conn.Object(busName, d.path) }

// SetTrusted marks the device as trusted so bluetoothd accepts reconnects from
// the watch without a new pairing agent round trip.
func (d *Device) SetTrusted(v bool) error {
	return d.bus.setProp(d.path, ifaceDevice, "Trusted", v)
}

// Pair performs bonding. A pairing agent must already be registered.
func (d *Device) Pair(ctx context.Context) error {
	if d.Paired() {
		return nil
	}
	call := d.obj().CallWithContext(ctx, ifaceDevice+".Pair", 0)
	if call.Err != nil && !strings.Contains(call.Err.Error(), "AlreadyExists") {
		return fmt.Errorf("ble: pair %s: %w", d.addr, call.Err)
	}
	return nil
}

// Connect establishes the ACL link and waits until the GATT database has been
// resolved, which is when characteristics become addressable.
func (d *Device) Connect(ctx context.Context) error {
	if !d.Connected() {
		if err := d.obj().CallWithContext(ctx, ifaceDevice+".Connect", 0).Err; err != nil {
			return fmt.Errorf("ble: connect %s: %w", d.addr, err)
		}
	}
	return d.waitServicesResolved(ctx)
}

func (d *Device) Disconnect() error {
	return d.obj().Call(ifaceDevice+".Disconnect", 0).Err
}

func (d *Device) waitServicesResolved(ctx context.Context) error {
	if boolProp(d.bus.prop(d.path, ifaceDevice, "ServicesResolved")) {
		return nil
	}
	done := make(chan struct{})
	var once sync.Once
	cancel := d.bus.subscribe(&subscription{
		path:  d.path,
		iface: ifaceDevice,
		fn: func(_ dbus.ObjectPath, _ string, changed map[string]dbus.Variant) {
			if v, ok := changed["ServicesResolved"]; ok {
				if b, _ := v.Value().(bool); b {
					once.Do(func() { close(done) })
				}
			}
		},
	})
	defer cancel()

	if boolProp(d.bus.prop(d.path, ifaceDevice, "ServicesResolved")) {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("ble: services not resolved for %s: %w", d.addr, ctx.Err())
	}
}

// OnDisconnect invokes fn when the ACL link drops. The returned func cancels.
func (d *Device) OnDisconnect(fn func()) (cancel func()) {
	return d.bus.subscribe(&subscription{
		path:  d.path,
		iface: ifaceDevice,
		fn: func(_ dbus.ObjectPath, _ string, changed map[string]dbus.Variant) {
			if v, ok := changed["Connected"]; ok {
				if b, _ := v.Value().(bool); !b {
					fn()
				}
			}
		},
	})
}

// UUIDs lists the GATT service UUIDs advertised or discovered for the device.
func (d *Device) UUIDs() []string {
	return strsProp(d.bus.prop(d.path, ifaceDevice, "UUIDs"))
}

// ServiceUUIDs lists the UUIDs of resolved GATT service objects.
func (d *Device) ServiceUUIDs() []string {
	paths := d.bus.paths(ifaceService, d.path)
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if u := strProp(d.bus.prop(p, ifaceService, "UUID")); u != "" {
			out = append(out, strings.ToLower(u))
		}
	}
	return out
}

// HasService reports whether the resolved GATT database contains uuid.
func (d *Device) HasService(uuid string) bool {
	uuid = strings.ToLower(uuid)
	for _, u := range d.ServiceUUIDs() {
		if u == uuid {
			return true
		}
	}
	return false
}

// Characteristic finds a characteristic by UUID anywhere under the device.
func (d *Device) Characteristic(uuid string) (*Char, error) {
	uuid = strings.ToLower(uuid)
	for _, p := range d.bus.paths(ifaceChar, d.path) {
		if strings.ToLower(strProp(d.bus.prop(p, ifaceChar, "UUID"))) == uuid {
			return &Char{bus: d.bus, path: p, uuid: uuid}, nil
		}
	}
	return nil, fmt.Errorf("ble: characteristic %s on %s: %w", uuid, d.addr, ErrNotFound)
}

// CharacteristicIn finds a characteristic by UUID inside one specific service,
// which matters for Garmin devices that expose the same UUID more than once.
func (d *Device) CharacteristicIn(serviceUUID, charUUID string) (*Char, error) {
	serviceUUID, charUUID = strings.ToLower(serviceUUID), strings.ToLower(charUUID)
	for _, sp := range d.bus.paths(ifaceService, d.path) {
		if strings.ToLower(strProp(d.bus.prop(sp, ifaceService, "UUID"))) != serviceUUID {
			continue
		}
		for _, cp := range d.bus.paths(ifaceChar, sp) {
			if strings.ToLower(strProp(d.bus.prop(cp, ifaceChar, "UUID"))) == charUUID {
				return &Char{bus: d.bus, path: cp, uuid: charUUID}, nil
			}
		}
	}
	return nil, fmt.Errorf("ble: characteristic %s in service %s: %w", charUUID, serviceUUID, ErrNotFound)
}

// WaitForCharacteristic polls until the characteristic shows up; BlueZ exports
// GATT objects incrementally right after ServicesResolved on some kernels.
func (d *Device) WaitForCharacteristic(ctx context.Context, serviceUUID, charUUID string) (*Char, error) {
	for {
		c, err := d.CharacteristicIn(serviceUUID, charUUID)
		if err == nil {
			return c, nil
		}
		select {
		case <-ctx.Done():
			return nil, err
		case <-time.After(200 * time.Millisecond):
		}
	}
}
