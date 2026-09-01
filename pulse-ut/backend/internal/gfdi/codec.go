package gfdi

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// Everything on the wire is little-endian; strings are a uint8 length followed
// by UTF-8 bytes.

var (
	// ErrShortRead is returned when a payload ends mid-field.
	ErrShortRead = errors.New("gfdi: short read")
	// ErrBadCRC is returned when a received frame fails its checksum.
	ErrBadCRC = errors.New("gfdi: bad crc")
	// ErrBadLength is returned when the declared frame length disagrees with
	// the received buffer.
	ErrBadLength = errors.New("gfdi: bad length")
)

// Writer builds a message payload.
type Writer struct{ buf []byte }

func NewWriter() *Writer { return &Writer{buf: make([]byte, 0, 64)} }

func (w *Writer) Bytes() []byte { return w.buf }
func (w *Writer) Len() int      { return len(w.buf) }

func (w *Writer) U8(v uint8)   { w.buf = append(w.buf, v) }
func (w *Writer) I8(v int8)    { w.buf = append(w.buf, byte(v)) }
func (w *Writer) Raw(b []byte) { w.buf = append(w.buf, b...) }

func (w *Writer) U16(v uint16) { w.buf = binary.LittleEndian.AppendUint16(w.buf, v) }
func (w *Writer) U32(v uint32) { w.buf = binary.LittleEndian.AppendUint32(w.buf, v) }
func (w *Writer) U64(v uint64) { w.buf = binary.LittleEndian.AppendUint64(w.buf, v) }
func (w *Writer) I16(v int16)  { w.U16(uint16(v)) }
func (w *Writer) I32(v int32)  { w.U32(uint32(v)) }
func (w *Writer) I64(v int64)  { w.U64(uint64(v)) }

func (w *Writer) F32(v float32) { w.U32(math.Float32bits(v)) }
func (w *Writer) F64(v float64) { w.U64(math.Float64bits(v)) }

// Str writes a uint8-length-prefixed UTF-8 string, truncating at 255 bytes.
func (w *Writer) Str(s string) {
	b := []byte(s)
	if len(b) > 255 {
		b = b[:255]
	}
	w.U8(uint8(len(b)))
	w.Raw(b)
}

// CStr writes a NUL-terminated UTF-8 string.
func (w *Writer) CStr(s string) {
	w.Raw([]byte(s))
	w.U8(0)
}

// Reader consumes a message payload.
type Reader struct {
	buf []byte
	pos int
}

func NewReader(b []byte) *Reader { return &Reader{buf: b} }

func (r *Reader) Remaining() int { return len(r.buf) - r.pos }
func (r *Reader) Pos() int       { return r.pos }
func (r *Reader) Rest() []byte   { b := r.buf[r.pos:]; r.pos = len(r.buf); return b }

func (r *Reader) take(n int) ([]byte, error) {
	if r.pos+n > len(r.buf) {
		return nil, ErrShortRead
	}
	b := r.buf[r.pos : r.pos+n]
	r.pos += n
	return b, nil
}

func (r *Reader) U8() (uint8, error) {
	b, err := r.take(1)
	if err != nil {
		return 0, err
	}
	return b[0], nil
}

func (r *Reader) U16() (uint16, error) {
	b, err := r.take(2)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(b), nil
}

func (r *Reader) U32() (uint32, error) {
	b, err := r.take(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(b), nil
}

func (r *Reader) U64() (uint64, error) {
	b, err := r.take(8)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(b), nil
}

func (r *Reader) I16() (int16, error) { v, err := r.U16(); return int16(v), err }
func (r *Reader) I32() (int32, error) { v, err := r.U32(); return int32(v), err }
func (r *Reader) I64() (int64, error) { v, err := r.U64(); return int64(v), err }

// Str reads a uint8-length-prefixed UTF-8 string.
func (r *Reader) Str() (string, error) {
	n, err := r.U8()
	if err != nil {
		return "", err
	}
	b, err := r.take(int(n))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CStr reads a NUL-terminated UTF-8 string.
func (r *Reader) CStr() (string, error) {
	start := r.pos
	for r.pos < len(r.buf) {
		if r.buf[r.pos] == 0 {
			s := string(r.buf[start:r.pos])
			r.pos++
			return s, nil
		}
		r.pos++
	}
	return "", ErrShortRead
}

// BuildFrame wraps a payload into a complete GFDI frame:
//
//	uint16 size (whole frame incl. size and crc) | uint16 messageType | payload | uint16 crc
func BuildFrame(messageType uint16, payload []byte) []byte {
	total := 2 + 2 + len(payload) + 2
	frame := make([]byte, 0, total)
	frame = binary.LittleEndian.AppendUint16(frame, uint16(total))
	frame = binary.LittleEndian.AppendUint16(frame, messageType)
	frame = append(frame, payload...)
	frame = binary.LittleEndian.AppendUint16(frame, CRC16(0, frame))
	return frame
}

// Frame is a validated inbound GFDI frame.
type Frame struct {
	Type    uint16 // normalised message id (0x8000 form already expanded)
	Raw     uint16 // message type field exactly as received
	Payload []byte // payload with the CRC stripped
}

// ParseFrame validates length and CRC and normalises the message type. Watches
// use a short form where bit 15 is set: the low byte plus 5000 is the real id
// and bits 8..14 carry a sequence number that upstream ignores.
func ParseFrame(b []byte) (*Frame, error) {
	if len(b) < 6 {
		return nil, fmt.Errorf("%w: %d bytes", ErrBadLength, len(b))
	}
	size := int(binary.LittleEndian.Uint16(b[0:2]))
	if size != len(b) {
		return nil, fmt.Errorf("%w: declared %d, got %d", ErrBadLength, size, len(b))
	}
	want := binary.LittleEndian.Uint16(b[size-2:])
	if got := CRC16(0, b[:size-2]); got != want {
		return nil, fmt.Errorf("%w: got %04x want %04x", ErrBadCRC, got, want)
	}
	raw := binary.LittleEndian.Uint16(b[2:4])
	typ := raw
	if raw&0x8000 != 0 {
		typ = (raw & 0xFF) + 5000
	}
	payload := make([]byte, size-6)
	copy(payload, b[4:size-2])
	return &Frame{Type: typ, Raw: raw, Payload: payload}, nil
}
