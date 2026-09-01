package garmin

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"pulse/backend/internal/ble"
	"pulse/backend/internal/gfdi"
)

// Garmin exposes three generations of the GFDI carrier. Detection is by GATT
// characteristic presence after service discovery, never by advertisement.
const (
	uuidServiceV0 = "9b012401-bc30-ce9a-e111-0f67e491abde"
	uuidSendV0    = "df334c80-e6a7-d082-274d-78fc66f85e16"
	uuidRecvV0    = "4acbcd28-7425-868e-f447-915c8f00d0cb"

	uuidServiceV1 = "6a4e2401-667b-11e3-949a-0800200c9a66"
	uuidSendV1    = "6a4e4c80-667b-11e3-949a-0800200c9a66"
	uuidRecvV1    = "6a4ecd28-667b-11e3-949a-0800200c9a66"

	uuidServiceV2 = "6a4e2800-667b-11e3-949a-0800200c9a66"
)

func v2UUID(short uint16) string {
	return fmt.Sprintf("6a4e%04x-667b-11e3-949a-0800200c9a66", short)
}

// Multi-link control channel constants.
const (
	mlClientID = int64(2)

	mlRegisterReq  = 0
	mlRegisterResp = 1
	mlCloseReq     = 2
	mlCloseResp    = 3
	mlCloseAllReq  = 5
	mlCloseAllResp = 6

	mlServiceGFDI = uint16(1)
)

// Transport carries complete GFDI frames over BLE.
type Transport interface {
	// Send transmits one complete GFDI frame.
	Send(frame []byte) error
	// Frames yields decoded inbound GFDI frames.
	Frames() <-chan []byte
	// Version reports "v0", "v1" or "v2" for logging and diagnostics.
	Version() string
	Close() error
}

// OpenTransport probes the connected device and returns the right carrier.
// V2 (multi-link) wins when present, which is the case for every recent watch
// including the Forerunner 255.
func OpenTransport(ctx context.Context, dev *ble.Device, log *slog.Logger) (Transport, error) {
	if t, err := openV2(ctx, dev, log); err == nil {
		return t, nil
	} else if !isNotFound(err) {
		return nil, err
	}
	if t, err := openV1(ctx, dev, log, uuidServiceV1, uuidSendV1, uuidRecvV1, "v1"); err == nil {
		return t, nil
	} else if !isNotFound(err) {
		return nil, err
	}
	t, err := openV1(ctx, dev, log, uuidServiceV0, uuidSendV0, uuidRecvV0, "v0")
	if err != nil {
		return nil, fmt.Errorf("garmin: no GFDI service on %s: %w", dev.Address(), err)
	}
	return t, nil
}

func isNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), ble.ErrNotFound.Error())
}

// maxWriteChunk mirrors calcMaxWriteChunk: min(512, max(23, mtu) - 3).
func maxWriteChunk(mtu uint16) int {
	m := int(mtu)
	if m < 23 {
		m = 23
	}
	m -= 3
	if m > 512 {
		m = 512
	}
	return m
}

// ---------------------------------------------------------------- v1 / v0 ---

type transportV1 struct {
	version string
	log     *slog.Logger
	send    *ble.Char
	recv    *ble.Char
	stop    func()

	writeMu sync.Mutex
	dec     *gfdi.CobsDecoder
	decMu   sync.Mutex
	out     chan []byte
	closed  chan struct{}
	once    sync.Once
}

func openV1(ctx context.Context, dev *ble.Device, log *slog.Logger, service, sendUUID, recvUUID, version string) (Transport, error) {
	sendCh, err := dev.CharacteristicIn(service, sendUUID)
	if err != nil {
		return nil, err
	}
	recvCh, err := dev.CharacteristicIn(service, recvUUID)
	if err != nil {
		return nil, err
	}
	t := &transportV1{
		version: version,
		log:     log,
		send:    sendCh,
		recv:    recvCh,
		dec:     gfdi.NewCobsDecoder(),
		out:     make(chan []byte, 64),
		closed:  make(chan struct{}),
	}
	stop, err := recvCh.Notify(t.onNotify)
	if err != nil {
		return nil, err
	}
	t.stop = stop
	log.Info("garmin transport open", "version", version, "mtu", recvCh.MTU())
	return t, nil
}

func (t *transportV1) onNotify(b []byte) {
	t.decMu.Lock()
	frames := t.dec.Feed(b)
	t.decMu.Unlock()
	for _, f := range frames {
		t.emit(f)
	}
}

func (t *transportV1) emit(f []byte) {
	select {
	case t.out <- f:
	case <-t.closed:
	default:
		t.log.Warn("garmin transport: inbound queue full, dropping frame")
	}
}

func (t *transportV1) Send(frame []byte) error {
	payload := gfdi.EncodeCOBS(frame)
	chunk := maxWriteChunk(t.send.MTU()) - 1
	if chunk < 19 {
		chunk = 19
	}
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	for off := 0; off < len(payload); off += chunk {
		end := off + chunk
		if end > len(payload) {
			end = len(payload)
		}
		if err := t.send.Write(payload[off:end]); err != nil {
			return err
		}
	}
	return nil
}

func (t *transportV1) Frames() <-chan []byte { return t.out }
func (t *transportV1) Version() string       { return t.version }

func (t *transportV1) Close() error {
	t.once.Do(func() {
		close(t.closed)
		if t.stop != nil {
			t.stop()
		}
		t.send.Close()
		t.recv.Close()
	})
	return nil
}

// --------------------------------------------------------------------- v2 ---

type transportV2 struct {
	log  *slog.Logger
	send *ble.Char
	recv *ble.Char
	stop func()

	writeMu sync.Mutex
	decMu   sync.Mutex
	dec     *gfdi.CobsDecoder

	mu         sync.Mutex
	gfdiHandle uint8
	haveHandle bool
	closingAll bool
	registered chan struct{}

	out    chan []byte
	closed chan struct{}
	once   sync.Once
}

func openV2(ctx context.Context, dev *ble.Device, log *slog.Logger) (Transport, error) {
	var sendCh, recvCh *ble.Char
	var lastErr error
	for short := uint16(0x2810); short <= 0x2814; short++ {
		r, err := dev.CharacteristicIn(uuidServiceV2, v2UUID(short))
		if err != nil {
			lastErr = err
			continue
		}
		s, err := dev.CharacteristicIn(uuidServiceV2, v2UUID(short+0x10))
		if err != nil {
			lastErr = err
			continue
		}
		recvCh, sendCh = r, s
		break
	}
	if sendCh == nil || recvCh == nil {
		if lastErr == nil {
			lastErr = ble.ErrNotFound
		}
		return nil, lastErr
	}

	t := &transportV2{
		log:        log,
		send:       sendCh,
		recv:       recvCh,
		dec:        gfdi.NewCobsDecoder(),
		registered: make(chan struct{}),
		out:        make(chan []byte, 64),
		closed:     make(chan struct{}),
	}
	stop, err := recvCh.Notify(t.onNotify)
	if err != nil {
		return nil, err
	}
	t.stop = stop

	// Clear any channels a previous host left registered, then take GFDI.
	t.mu.Lock()
	t.closingAll = true
	t.mu.Unlock()
	if err := t.writeRaw(mlControlPacket(mlCloseAllReq, 0, 0)); err != nil {
		stop()
		return nil, fmt.Errorf("garmin: close-all: %w", err)
	}

	select {
	case <-t.registered:
	case <-ctx.Done():
		stop()
		return nil, fmt.Errorf("garmin: GFDI channel registration timed out: %w", ctx.Err())
	}
	log.Info("garmin transport open", "version", "v2", "handle", t.gfdiHandle, "mtu", recvCh.MTU())
	return t, nil
}

// mlControlPacket builds one of the three 13 byte control packets on handle 0.
func mlControlPacket(reqType uint8, serviceCode uint16, trailer uint8) []byte {
	b := make([]byte, 0, 13)
	b = append(b, 0x00, reqType)
	b = binary.LittleEndian.AppendUint64(b, uint64(mlClientID))
	b = binary.LittleEndian.AppendUint16(b, serviceCode)
	b = append(b, trailer)
	return b
}

func (t *transportV2) writeRaw(b []byte) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	return t.send.Write(b)
}

func (t *transportV2) onNotify(v []byte) {
	if len(v) == 0 {
		return
	}
	handle := v[0]
	// Bit 7 marks a reliable (MLR) channel. We never register reliable
	// channels, and issue #5476 in upstream shows plain handles can carry the
	// bit too, so mask it off and route by the low bits.
	if handle&0x80 != 0 {
		handle = (handle & 0x70) >> 4
	}
	if handle == 0 {
		t.onControl(v)
		return
	}
	t.mu.Lock()
	want, ok := t.gfdiHandle, t.haveHandle
	t.mu.Unlock()
	if !ok || handle != want {
		return
	}
	t.decMu.Lock()
	frames := t.dec.Feed(v[1:])
	t.decMu.Unlock()
	for _, f := range frames {
		select {
		case t.out <- f:
		case <-t.closed:
			return
		default:
			t.log.Warn("garmin transport: inbound queue full, dropping frame")
		}
	}
}

func (t *transportV2) onControl(v []byte) {
	if len(v) < 10 {
		return
	}
	reqType := v[1]
	if int64(binary.LittleEndian.Uint64(v[2:10])) != mlClientID {
		return
	}
	body := v[10:]

	switch reqType {
	case mlCloseAllResp:
		t.mu.Lock()
		t.closingAll = false
		t.haveHandle = false
		t.mu.Unlock()
		t.decMu.Lock()
		t.dec.Reset()
		t.decMu.Unlock()
		t.registerGFDI()

	case mlRegisterResp:
		if len(body) < 4 {
			return
		}
		service := binary.LittleEndian.Uint16(body[0:2])
		status := body[2]
		handle := body[3]
		if service != mlServiceGFDI {
			return
		}
		if status != 0 {
			t.log.Error("garmin: GFDI registration refused", "status", status)
			return
		}
		t.mu.Lock()
		t.gfdiHandle = handle
		t.haveHandle = true
		t.mu.Unlock()
		t.decMu.Lock()
		t.dec.Reset()
		t.decMu.Unlock()
		select {
		case <-t.registered:
		default:
			close(t.registered)
		}

	case mlCloseResp:
		if len(body) < 4 {
			return
		}
		service := binary.LittleEndian.Uint16(body[0:2])
		t.mu.Lock()
		closing := t.closingAll
		if service == mlServiceGFDI {
			t.haveHandle = false
		}
		t.mu.Unlock()
		if service == mlServiceGFDI && !closing {
			// The watch closed our channel; take it back immediately.
			t.registerGFDI()
		}
	}
}

func (t *transportV2) registerGFDI() {
	// reliable = 0: plain multi-link. MLR adds retransmission on top and is
	// off by default upstream too.
	if err := t.writeRaw(mlControlPacket(mlRegisterReq, mlServiceGFDI, 0)); err != nil {
		t.log.Error("garmin: register GFDI failed", "err", err)
	}
}

func (t *transportV2) Send(frame []byte) error {
	t.mu.Lock()
	handle, ok := t.gfdiHandle, t.haveHandle
	t.mu.Unlock()
	if !ok {
		return fmt.Errorf("garmin: GFDI channel not registered")
	}

	payload := gfdi.EncodeCOBS(frame)
	chunk := maxWriteChunk(t.send.MTU()) - 1
	if chunk < 19 {
		chunk = 19
	}
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	buf := make([]byte, 0, chunk+1)
	for off := 0; off < len(payload); off += chunk {
		end := off + chunk
		if end > len(payload) {
			end = len(payload)
		}
		buf = append(buf[:0], handle)
		buf = append(buf, payload[off:end]...)
		if err := t.send.Write(buf); err != nil {
			return err
		}
	}
	return nil
}

func (t *transportV2) Frames() <-chan []byte { return t.out }
func (t *transportV2) Version() string       { return "v2" }

func (t *transportV2) Close() error {
	t.once.Do(func() {
		close(t.closed)
		t.mu.Lock()
		handle, ok := t.gfdiHandle, t.haveHandle
		t.mu.Unlock()
		if ok {
			_ = t.writeRaw(mlControlPacket(mlCloseReq, mlServiceGFDI, handle))
			time.Sleep(50 * time.Millisecond)
		}
		if t.stop != nil {
			t.stop()
		}
		t.send.Close()
		t.recv.Close()
	})
	return nil
}
