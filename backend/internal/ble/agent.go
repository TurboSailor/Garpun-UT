package ble

import (
	"errors"
	"fmt"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
)

const agentPath = dbus.ObjectPath("/cc/zachy/pulse/agent")

// ErrPairingRejected aborts an in-flight pairing.
var ErrPairingRejected = errors.New("ble: pairing rejected")

// PairingUI is implemented by whatever surface can talk to the user. Garmin
// watches show a six digit code and expect the phone to type it back, which is
// RequestPasskey; some models instead use numeric comparison
// (RequestConfirmation).
type PairingUI interface {
	// RequestPasskey asks the user for the code displayed on the watch.
	RequestPasskey(device string) (uint32, error)
	// RequestConfirmation asks the user to confirm a matching code.
	RequestConfirmation(device string, passkey uint32) error
	// DisplayPasskey reports a code the user must confirm on the watch.
	DisplayPasskey(device string, passkey uint32, entered uint16)
	// Cancel tells the UI the request went away.
	Cancel()
}

// Agent implements org.bluez.Agent1 and registers itself as the default agent.
type Agent struct {
	bus *Bus
	ui  PairingUI

	mu         sync.Mutex
	registered bool
}

// NewAgent exports the agent object on the bus. Call Register to make BlueZ use
// it. Capability must be one of DisplayOnly, DisplayYesNo, KeyboardOnly,
// NoInputNoOutput, KeyboardDisplay.
func NewAgent(bus *Bus, ui PairingUI) (*Agent, error) {
	a := &Agent{bus: bus, ui: ui}
	conn := bus.Conn()

	if err := conn.Export(a, agentPath, "org.bluez.Agent1"); err != nil {
		return nil, fmt.Errorf("ble: export agent: %w", err)
	}
	node := &introspect.Node{
		Name: string(agentPath),
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			prop.IntrospectData,
			{
				Name: "org.bluez.Agent1",
				Methods: []introspect.Method{
					{Name: "Release"},
					{Name: "RequestPinCode", Args: []introspect.Arg{{Name: "device", Type: "o", Direction: "in"}, {Name: "pincode", Type: "s", Direction: "out"}}},
					{Name: "DisplayPinCode", Args: []introspect.Arg{{Name: "device", Type: "o", Direction: "in"}, {Name: "pincode", Type: "s", Direction: "in"}}},
					{Name: "RequestPasskey", Args: []introspect.Arg{{Name: "device", Type: "o", Direction: "in"}, {Name: "passkey", Type: "u", Direction: "out"}}},
					{Name: "DisplayPasskey", Args: []introspect.Arg{{Name: "device", Type: "o", Direction: "in"}, {Name: "passkey", Type: "u", Direction: "in"}, {Name: "entered", Type: "q", Direction: "in"}}},
					{Name: "RequestConfirmation", Args: []introspect.Arg{{Name: "device", Type: "o", Direction: "in"}, {Name: "passkey", Type: "u", Direction: "in"}}},
					{Name: "RequestAuthorization", Args: []introspect.Arg{{Name: "device", Type: "o", Direction: "in"}}},
					{Name: "AuthorizeService", Args: []introspect.Arg{{Name: "device", Type: "o", Direction: "in"}, {Name: "uuid", Type: "s", Direction: "in"}}},
					{Name: "Cancel"},
				},
			},
		},
	}
	if err := conn.Export(introspect.NewIntrospectable(node), agentPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		return nil, fmt.Errorf("ble: export introspection: %w", err)
	}
	return a, nil
}

// Register installs the agent as the system default pairing agent.
func (a *Agent) Register(capability string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.registered {
		return nil
	}
	mgr := a.bus.conn.Object(busName, "/org/bluez")
	if err := mgr.Call(ifaceAgentManager+".RegisterAgent", 0, agentPath, capability).Err; err != nil {
		return fmt.Errorf("ble: RegisterAgent: %w", err)
	}
	if err := mgr.Call(ifaceAgentManager+".RequestDefaultAgent", 0, agentPath).Err; err != nil {
		return fmt.Errorf("ble: RequestDefaultAgent: %w", err)
	}
	a.registered = true
	return nil
}

// Unregister removes the agent, handing control back to the system UI.
func (a *Agent) Unregister() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.registered {
		return
	}
	a.bus.conn.Object(busName, "/org/bluez").
		Call(ifaceAgentManager+".UnregisterAgent", 0, agentPath)
	a.registered = false
}

func (a *Agent) addr(device dbus.ObjectPath) string {
	return strProp(a.bus.prop(device, ifaceDevice, "Address"))
}

func rejected(err error) *dbus.Error {
	return dbus.NewError("org.bluez.Error.Rejected", []any{err.Error()})
}

func (a *Agent) Release() *dbus.Error { return nil }

func (a *Agent) Cancel() *dbus.Error {
	a.ui.Cancel()
	return nil
}

func (a *Agent) RequestPinCode(device dbus.ObjectPath) (string, *dbus.Error) {
	key, err := a.ui.RequestPasskey(a.addr(device))
	if err != nil {
		return "", rejected(err)
	}
	return fmt.Sprintf("%06d", key), nil
}

func (a *Agent) DisplayPinCode(device dbus.ObjectPath, pincode string) *dbus.Error {
	var key uint32
	fmt.Sscanf(pincode, "%d", &key)
	a.ui.DisplayPasskey(a.addr(device), key, 0)
	return nil
}

func (a *Agent) RequestPasskey(device dbus.ObjectPath) (uint32, *dbus.Error) {
	key, err := a.ui.RequestPasskey(a.addr(device))
	if err != nil {
		return 0, rejected(err)
	}
	return key, nil
}

func (a *Agent) DisplayPasskey(device dbus.ObjectPath, passkey uint32, entered uint16) *dbus.Error {
	a.ui.DisplayPasskey(a.addr(device), passkey, entered)
	return nil
}

func (a *Agent) RequestConfirmation(device dbus.ObjectPath, passkey uint32) *dbus.Error {
	if err := a.ui.RequestConfirmation(a.addr(device), passkey); err != nil {
		return rejected(err)
	}
	return nil
}

func (a *Agent) RequestAuthorization(device dbus.ObjectPath) *dbus.Error { return nil }

func (a *Agent) AuthorizeService(device dbus.ObjectPath, uuid string) *dbus.Error { return nil }
