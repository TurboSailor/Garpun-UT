package ble

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
)

// Char wraps org.bluez.GattCharacteristic1.
//
// Two IO paths are supported. The D-Bus path (WriteValue/StartNotify) always
// works; the socket path (AcquireWrite/AcquireNotify) hands back a SeqPacket fd
// and removes one round trip per ATT packet, which matters a lot when pulling
// multi-megabyte FIT files off a watch.
type Char struct {
	bus  *Bus
	path dbus.ObjectPath
	uuid string

	mu       sync.Mutex
	writeFD  *os.File
	writeMTU uint16
	notifyFD *os.File
	stopSub  func()
}

func (c *Char) UUID() string          { return c.uuid }
func (c *Char) Path() dbus.ObjectPath { return c.path }
func (c *Char) obj() dbus.BusObject   { return c.bus.conn.Object(busName, c.path) }
func (c *Char) Flags() []string       { return strsProp(c.bus.prop(c.path, ifaceChar, "Flags")) }

func (c *Char) hasFlag(f string) bool {
	for _, v := range c.Flags() {
		if v == f {
			return true
		}
	}
	return false
}

// MTU reports the negotiated ATT payload size for this characteristic, or 0 if
// BlueZ has not exposed it yet.
func (c *Char) MTU() uint16 {
	v, ok := c.bus.prop(c.path, ifaceChar, "MTU")
	if !ok {
		return 0
	}
	n, _ := v.Value().(uint16)
	return n
}

// Read performs an ATT read.
func (c *Char) Read() ([]byte, error) {
	var out []byte
	opts := map[string]dbus.Variant{}
	if err := c.obj().Call(ifaceChar+".ReadValue", 0, opts).Store(&out); err != nil {
		return nil, fmt.Errorf("ble: read %s: %w", c.uuid, err)
	}
	return out, nil
}

// Write sends one ATT payload. It uses the acquired write socket when
// available, otherwise WriteValue with the strongest type the characteristic
// supports.
//
// BlueZ answers "In Progress" while an earlier ATT operation on the same
// connection is still outstanding, which happens on bursts of GFDI frames.
// That is back pressure, not a failure, so the write is retried briefly. A
// dropped link, in contrast, is reported as ErrNotConnected and must end the
// session.
func (c *Char) Write(b []byte) error {
	c.mu.Lock()
	fd := c.writeFD
	c.mu.Unlock()
	if fd != nil {
		if _, err := fd.Write(b); err != nil {
			if errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ENOTCONN) {
				return fmt.Errorf("ble: write sock %s: %w", c.uuid, ErrNotConnected)
			}
			return fmt.Errorf("ble: write sock %s: %w", c.uuid, err)
		}
		return nil
	}

	typ := "request"
	if !c.hasFlag("write") && c.hasFlag("write-without-response") {
		typ = "command"
	}
	opts := map[string]dbus.Variant{"type": dbus.MakeVariant(typ)}
	const busyRetries = 8
	for attempt := 0; ; attempt++ {
		err := c.obj().Call(ifaceChar+".WriteValue", 0, b, opts).Err
		if err == nil {
			return nil
		}
		switch dbusErrorName(err) {
		case "org.bluez.Error.InProgress":
			if attempt < busyRetries {
				time.Sleep(25 * time.Millisecond)
				continue
			}
		case "org.bluez.Error.NotConnected", "org.bluez.Error.Failed":
			// Failed covers "Not connected" reported by older bluetoothd.
			if strings.Contains(err.Error(), "onnected") {
				return fmt.Errorf("ble: write %s: %w", c.uuid, ErrNotConnected)
			}
		}
		return fmt.Errorf("ble: write %s: %w", c.uuid, err)
	}
}

// dbusErrorName reports the D-Bus error name of err, or "".
func dbusErrorName(err error) string {
	var dberr dbus.Error
	if errors.As(err, &dberr) {
		return dberr.Name
	}
	return ""
}

// AcquireWrite switches Write to the socket fast path. Returns the usable
// payload size. Safe to call on characteristics that do not support it: the
// error is returned and the D-Bus path stays in use.
func (c *Char) AcquireWrite() (uint16, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.writeFD != nil {
		return c.writeMTU, nil
	}
	var fd dbus.UnixFD
	var mtu uint16
	opts := map[string]dbus.Variant{}
	if err := c.obj().Call(ifaceChar+".AcquireWrite", 0, opts).Store(&fd, &mtu); err != nil {
		return 0, fmt.Errorf("ble: AcquireWrite %s: %w", c.uuid, err)
	}
	c.writeFD = os.NewFile(uintptr(fd), "gatt-write-"+c.uuid)
	c.writeMTU = mtu
	return mtu, nil
}

// Notify subscribes to notifications. fn is called for every ATT notification
// with the raw payload; the returned func unsubscribes.
func (c *Char) Notify(fn func([]byte)) (func(), error) {
	c.mu.Lock()
	if c.stopSub != nil {
		c.mu.Unlock()
		return nil, fmt.Errorf("ble: %s already subscribed", c.uuid)
	}
	c.mu.Unlock()

	cancel := c.bus.subscribe(&subscription{
		path:  c.path,
		iface: ifaceChar,
		fn: func(_ dbus.ObjectPath, _ string, changed map[string]dbus.Variant) {
			v, ok := changed["Value"]
			if !ok {
				return
			}
			if b, ok := v.Value().([]byte); ok && len(b) > 0 {
				buf := make([]byte, len(b))
				copy(buf, b)
				fn(buf)
			}
		},
	})

	if err := c.obj().Call(ifaceChar+".StartNotify", 0).Err; err != nil {
		cancel()
		return nil, fmt.Errorf("ble: StartNotify %s: %w", c.uuid, err)
	}

	stop := func() {
		cancel()
		c.obj().Call(ifaceChar+".StopNotify", 0)
		c.mu.Lock()
		c.stopSub = nil
		c.mu.Unlock()
	}
	c.mu.Lock()
	c.stopSub = stop
	c.mu.Unlock()
	return stop, nil
}

// NotifySocket subscribes using AcquireNotify and pumps the resulting
// SeqPacket socket. Each read yields exactly one notification payload.
func (c *Char) NotifySocket(fn func([]byte)) (func(), error) {
	var fd dbus.UnixFD
	var mtu uint16
	opts := map[string]dbus.Variant{}
	if err := c.obj().Call(ifaceChar+".AcquireNotify", 0, opts).Store(&fd, &mtu); err != nil {
		return nil, fmt.Errorf("ble: AcquireNotify %s: %w", c.uuid, err)
	}
	f := os.NewFile(uintptr(fd), "gatt-notify-"+c.uuid)
	c.mu.Lock()
	c.notifyFD = f
	c.mu.Unlock()

	if mtu == 0 {
		mtu = 512
	}
	go func() {
		buf := make([]byte, int(mtu)+3)
		for {
			n, err := f.Read(buf)
			if err != nil {
				return
			}
			if n <= 0 {
				continue
			}
			out := make([]byte, n)
			copy(out, buf[:n])
			fn(out)
		}
	}()

	return func() {
		c.mu.Lock()
		if c.notifyFD != nil {
			c.notifyFD.Close()
			c.notifyFD = nil
		}
		c.mu.Unlock()
	}, nil
}

// Close releases any acquired sockets.
func (c *Char) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.writeFD != nil {
		c.writeFD.Close()
		c.writeFD = nil
	}
	if c.notifyFD != nil {
		c.notifyFD.Close()
		c.notifyFD = nil
	}
}
