package ble

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// Adapter wraps org.bluez.Adapter1.
type Adapter struct {
	bus  *Bus
	path dbus.ObjectPath
}

// DefaultAdapter returns the first adapter reported by BlueZ (usually hci0).
func (b *Bus) DefaultAdapter() (*Adapter, error) {
	paths := b.paths(ifaceAdapter, "")
	if len(paths) == 0 {
		return nil, fmt.Errorf("ble: no bluetooth adapter: %w", ErrNotFound)
	}
	sort.Slice(paths, func(i, j int) bool { return paths[i] < paths[j] })
	return &Adapter{bus: b, path: paths[0]}, nil
}

func (a *Adapter) Path() dbus.ObjectPath { return a.path }

func (a *Adapter) Address() string {
	return strProp(a.bus.prop(a.path, ifaceAdapter, "Address"))
}

func (a *Adapter) Powered() bool {
	return boolProp(a.bus.prop(a.path, ifaceAdapter, "Powered"))
}

// SetPowered turns the controller on or off and waits for the property to
// settle, because bluetoothd applies it asynchronously.
func (a *Adapter) SetPowered(ctx context.Context, on bool) error {
	if a.Powered() == on {
		return nil
	}
	if err := a.bus.setProp(a.path, ifaceAdapter, "Powered", on); err != nil {
		return fmt.Errorf("ble: set Powered: %w", err)
	}
	return a.waitProp(ctx, ifaceAdapter, "Powered", on)
}

func (a *Adapter) waitProp(ctx context.Context, iface, name string, want any) error {
	done := make(chan struct{})
	var once bool
	cancel := a.bus.subscribe(&subscription{
		path:  a.path,
		iface: iface,
		fn: func(_ dbus.ObjectPath, _ string, changed map[string]dbus.Variant) {
			if v, ok := changed[name]; ok && v.Value() == want && !once {
				once = true
				close(done)
			}
		},
	})
	defer cancel()

	if v, ok := a.bus.prop(a.path, iface, name); ok && v.Value() == want {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// DeviceInfo is a discovery result.
type DeviceInfo struct {
	Address string   `json:"address"`
	Name    string   `json:"name"`
	Alias   string   `json:"alias"`
	RSSI    int16    `json:"rssi"`
	Paired  bool     `json:"paired"`
	Trusted bool     `json:"trusted"`
	Bonded  bool     `json:"bonded"`
	Conn    bool     `json:"connected"`
	UUIDs   []string `json:"uuids"`
}

func (a *Adapter) deviceInfo(path dbus.ObjectPath) DeviceInfo {
	p := func(n string) (dbus.Variant, bool) { return a.bus.prop(path, ifaceDevice, n) }
	return DeviceInfo{
		Address: strProp(p("Address")),
		Name:    strProp(p("Name")),
		Alias:   strProp(p("Alias")),
		RSSI:    int16Prop(p("RSSI")),
		Paired:  boolProp(p("Paired")),
		Trusted: boolProp(p("Trusted")),
		Bonded:  boolProp(p("Bonded")),
		Conn:    boolProp(p("Connected")),
		UUIDs:   strsProp(p("UUIDs")),
	}
}

// Devices lists every device object BlueZ currently knows about.
func (a *Adapter) Devices() []DeviceInfo {
	paths := a.bus.paths(ifaceDevice, a.path)
	out := make([]DeviceInfo, 0, len(paths))
	for _, p := range paths {
		out = append(out, a.deviceInfo(p))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RSSI > out[j].RSSI })
	return out
}

// Discover runs LE discovery for the given duration, invoking onFound for each
// device as it appears or updates. The callback also fires for already known
// devices so callers see a complete list.
func (a *Adapter) Discover(ctx context.Context, d time.Duration, onFound func(DeviceInfo)) error {
	filter := map[string]dbus.Variant{
		"Transport":     dbus.MakeVariant("le"),
		"DuplicateData": dbus.MakeVariant(false),
	}
	obj := a.bus.conn.Object(busName, a.path)
	if err := obj.Call(ifaceAdapter+".SetDiscoveryFilter", 0, filter).Err; err != nil {
		return fmt.Errorf("ble: SetDiscoveryFilter: %w", err)
	}

	emit := func(path dbus.ObjectPath) {
		if onFound == nil {
			return
		}
		if !strings.HasPrefix(string(path), string(a.path)+"/dev_") {
			return
		}
		onFound(a.deviceInfo(path))
	}

	cancel := a.bus.subscribe(&subscription{
		iface: ifaceDevice,
		fn:    func(path dbus.ObjectPath, _ string, _ map[string]dbus.Variant) { emit(path) },
		added: func(path dbus.ObjectPath, ifaces map[string]map[string]dbus.Variant) {
			if _, ok := ifaces[ifaceDevice]; ok {
				emit(path)
			}
		},
	})
	defer cancel()

	if err := obj.Call(ifaceAdapter+".StartDiscovery", 0).Err; err != nil {
		return fmt.Errorf("ble: StartDiscovery: %w", err)
	}
	defer obj.Call(ifaceAdapter+".StopDiscovery", 0)

	for _, p := range a.bus.paths(ifaceDevice, a.path) {
		emit(p)
	}

	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
	return nil
}

// devicePath maps AA:BB:.. to /org/bluez/hciN/dev_AA_BB_...
func (a *Adapter) devicePath(addr string) dbus.ObjectPath {
	return dbus.ObjectPath(string(a.path) + "/dev_" + strings.ReplaceAll(strings.ToUpper(addr), ":", "_"))
}

// Device returns a handle for a known device address.
func (a *Adapter) Device(addr string) (*Device, error) {
	path := a.devicePath(addr)
	if _, ok := a.bus.prop(path, ifaceDevice, "Address"); !ok {
		return nil, fmt.Errorf("ble: device %s: %w", addr, ErrNotFound)
	}
	return &Device{bus: a.bus, adapter: a, path: path, addr: strings.ToUpper(addr)}, nil
}

// RemoveDevice drops the pairing and cached GATT database for a device.
func (a *Adapter) RemoveDevice(addr string) error {
	return a.bus.conn.Object(busName, a.path).
		Call(ifaceAdapter+".RemoveDevice", 0, a.devicePath(addr)).Err
}
