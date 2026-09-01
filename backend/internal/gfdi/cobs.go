package gfdi

import "errors"

// Garmin frames the GFDI byte stream with a COBS variant that adds a leading
// 0x00 in addition to the usual trailing delimiter. The decoder must survive
// lost BLE notifications: a truncated or out-of-sync frame is dropped and the
// stream re-synchronises on the next complete frame.

const cobsBufferSize = 10000

var errTruncated = errors.New("gfdi: truncated cobs frame")

// EncodeCOBS wraps data as 0x00 <cobs> 0x00.
func EncodeCOBS(data []byte) []byte {
	out := make([]byte, 0, len(data)+len(data)/254+3)
	out = append(out, 0x00) // Garmin initial padding

	pos := 0
	lastWasZero := false
	for pos < len(data) {
		z := pos
		for z < len(data) && data[z] != 0x00 {
			z++
		}
		payload := z - pos
		for payload >= 0xFE {
			out = append(out, 0xFF)
			out = append(out, data[pos:pos+0xFE]...)
			payload -= 0xFE
			pos += 0xFE
		}
		out = append(out, byte(payload+1))
		out = append(out, data[pos:pos+payload]...)
		lastWasZero = z < len(data) // the zero byte at z is consumed by the code
		pos = z + 1
	}
	if lastWasZero {
		out = append(out, 0x01)
	}
	out = append(out, 0x00) // terminator
	return out
}

// CobsDecoder reassembles COBS frames from arbitrarily chopped BLE
// notifications. It is not safe for concurrent use; the transport owns one
// decoder per logical channel.
type CobsDecoder struct {
	buf []byte
}

// NewCobsDecoder returns a decoder with the same 10000 byte ceiling the Android
// implementation uses.
func NewCobsDecoder() *CobsDecoder {
	return &CobsDecoder{buf: make([]byte, 0, 512)}
}

// Reset drops all buffered bytes.
func (d *CobsDecoder) Reset() { d.buf = d.buf[:0] }

// Feed appends received bytes and returns every complete frame decoded so far.
func (d *CobsDecoder) Feed(chunk []byte) [][]byte {
	if len(d.buf)+len(chunk) > cobsBufferSize {
		// Overflow means the terminator was lost; drop and resync.
		d.buf = d.buf[:0]
	}
	d.buf = append(d.buf, chunk...)

	var out [][]byte
	for {
		frame, more := d.next()
		if frame != nil {
			out = append(out, frame)
		}
		if !more {
			return out
		}
	}
}

// next extracts one frame. more=true means the caller should call again.
func (d *CobsDecoder) next() (frame []byte, more bool) {
	if len(d.buf) < 4 {
		return nil, false
	}
	start := 0
	for start < len(d.buf) && d.buf[start] != 0x00 {
		start++
	}
	if start >= len(d.buf) {
		d.buf = d.buf[:0] // no frame delimiter at all: pure garbage
		return nil, false
	}
	end := start + 1
	for end < len(d.buf) && d.buf[end] != 0x00 {
		end++
	}
	if end >= len(d.buf) {
		if start > 0 {
			d.buf = append(d.buf[:0], d.buf[start:]...)
		}
		return nil, false
	}

	body := d.buf[start+1 : end]
	// Keep the trailing 0x00: it doubles as the leading pad of the next frame.
	d.buf = append(d.buf[:0], d.buf[end:]...)

	decoded, err := decodeCOBSBody(body)
	if err != nil {
		return nil, true // drop this frame, keep scanning
	}
	return decoded, true
}

func decodeCOBSBody(body []byte) ([]byte, error) {
	out := make([]byte, 0, len(body))
	i := 0
	for i < len(body) {
		code := int(body[i])
		i++
		if code == 0 {
			return nil, errTruncated
		}
		n := code - 1
		if i+n > len(body) {
			return nil, errTruncated
		}
		out = append(out, body[i:i+n]...)
		i += n
		if code != 0xFF && i < len(body) {
			out = append(out, 0x00)
		}
	}
	if len(out) == 0 {
		return nil, errTruncated
	}
	return out, nil
}
