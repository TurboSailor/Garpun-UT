package daemon

import (
	"fmt"
	"sync"
	"time"
)

// pairingRequest is an in-flight BlueZ agent callback waiting on the user.
type pairingRequest struct {
	Kind    string `json:"kind"` // "passkey" or "confirm"
	Address string `json:"address"`
	Passkey uint32 `json:"passkey"`

	reply chan pairingReply
}

type pairingReply struct {
	passkey uint32
	confirm bool
	cancel  bool
}

// PairingState is what the UI polls.
type PairingState struct {
	Pending bool   `json:"pending"`
	Kind    string `json:"kind,omitempty"`
	Address string `json:"address,omitempty"`
	Passkey uint32 `json:"passkey,omitempty"`
}

// pairingUI bridges BlueZ agent callbacks to the HTTP API. Garmin watches show
// a six digit code and expect the phone to type it back, so RequestPasskey has
// to block until the user answers.
type pairingUI struct {
	mu      sync.Mutex
	current *pairingRequest
	onState func(PairingState)
}

const pairingTimeout = 2 * time.Minute

func (p *pairingUI) begin(kind, address string, passkey uint32) *pairingRequest {
	req := &pairingRequest{
		Kind:    kind,
		Address: address,
		Passkey: passkey,
		reply:   make(chan pairingReply, 1),
	}
	p.mu.Lock()
	p.current = req
	notify := p.onState
	p.mu.Unlock()
	if notify != nil {
		notify(PairingState{Pending: true, Kind: kind, Address: address, Passkey: passkey})
	}
	return req
}

func (p *pairingUI) finish() {
	p.mu.Lock()
	p.current = nil
	notify := p.onState
	p.mu.Unlock()
	if notify != nil {
		notify(PairingState{})
	}
}

func (p *pairingUI) State() PairingState {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current == nil {
		return PairingState{}
	}
	return PairingState{
		Pending: true,
		Kind:    p.current.Kind,
		Address: p.current.Address,
		Passkey: p.current.Passkey,
	}
}

// Submit answers the pending request from the API.
func (p *pairingUI) Submit(r pairingReply) error {
	p.mu.Lock()
	req := p.current
	p.mu.Unlock()
	if req == nil {
		return fmt.Errorf("daemon: no pairing in progress")
	}
	select {
	case req.reply <- r:
		return nil
	default:
		return fmt.Errorf("daemon: pairing already answered")
	}
}

func (p *pairingUI) wait(req *pairingRequest) (pairingReply, error) {
	defer p.finish()
	select {
	case r := <-req.reply:
		if r.cancel {
			return r, fmt.Errorf("daemon: pairing cancelled")
		}
		return r, nil
	case <-time.After(pairingTimeout):
		return pairingReply{}, fmt.Errorf("daemon: pairing timed out")
	}
}

// RequestPasskey implements ble.PairingUI.
func (p *pairingUI) RequestPasskey(device string) (uint32, error) {
	req := p.begin("passkey", device, 0)
	r, err := p.wait(req)
	if err != nil {
		return 0, err
	}
	return r.passkey, nil
}

// RequestConfirmation implements ble.PairingUI.
func (p *pairingUI) RequestConfirmation(device string, passkey uint32) error {
	req := p.begin("confirm", device, passkey)
	r, err := p.wait(req)
	if err != nil {
		return err
	}
	if !r.confirm {
		return fmt.Errorf("daemon: pairing rejected")
	}
	return nil
}

// DisplayPasskey implements ble.PairingUI.
func (p *pairingUI) DisplayPasskey(device string, passkey uint32, entered uint16) {
	p.mu.Lock()
	notify := p.onState
	p.mu.Unlock()
	if notify != nil {
		notify(PairingState{Pending: true, Kind: "display", Address: device, Passkey: passkey})
	}
}

// Cancel implements ble.PairingUI.
func (p *pairingUI) Cancel() { p.finish() }
