package uxbridge

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

// A minimal adbd client. Waydroid keeps its Android notifications inside the
// container, so the only way to see them from the host session is to talk to
// the container's adbd directly. Pulling in a full adb implementation for two
// shell commands is not worth it; the wire protocol is a 24 byte header plus a
// payload and four message types.

const (
	adbHeaderSize = 24

	adbCNXN uint32 = 0x4e584e43
	adbAUTH uint32 = 0x48545541
	adbOPEN uint32 = 0x4e45504f
	adbOKAY uint32 = 0x59414b4f
	adbCLSE uint32 = 0x45534c43
	adbWRTE uint32 = 0x45545257

	// adbVersion 0x01000000 predates delayed acks, so adbd will not expect us
	// to advertise a per-stream window.
	adbVersion uint32 = 0x01000000
	adbMaxData uint32 = 256 * 1024

	adbBanner = "host::pulse\x00"
)

// errADBAuth means adbd wants an RSA key we do not have. Generating and
// getting one authorised needs a user tap inside the container, so callers
// fall back to running the shell command through waydroid instead.
var errADBAuth = errors.New("uxbridge: adbd requires authentication")

type adbHeader struct {
	command  uint32
	arg0     uint32
	arg1     uint32
	length   uint32
	checksum uint32
}

// adbChecksum is the plain byte sum adbd uses, not a real CRC32.
func adbChecksum(data []byte) uint32 {
	var sum uint32
	for _, b := range data {
		sum += uint32(b)
	}
	return sum
}

func encodeADBPacket(command, arg0, arg1 uint32, data []byte) []byte {
	buf := make([]byte, adbHeaderSize+len(data))
	binary.LittleEndian.PutUint32(buf[0:], command)
	binary.LittleEndian.PutUint32(buf[4:], arg0)
	binary.LittleEndian.PutUint32(buf[8:], arg1)
	binary.LittleEndian.PutUint32(buf[12:], uint32(len(data)))
	binary.LittleEndian.PutUint32(buf[16:], adbChecksum(data))
	binary.LittleEndian.PutUint32(buf[20:], ^command)
	copy(buf[adbHeaderSize:], data)
	return buf
}

func decodeADBHeader(b []byte) (adbHeader, error) {
	if len(b) < adbHeaderSize {
		return adbHeader{}, fmt.Errorf("uxbridge: short adb header (%d bytes)", len(b))
	}
	h := adbHeader{
		command:  binary.LittleEndian.Uint32(b[0:]),
		arg0:     binary.LittleEndian.Uint32(b[4:]),
		arg1:     binary.LittleEndian.Uint32(b[8:]),
		length:   binary.LittleEndian.Uint32(b[12:]),
		checksum: binary.LittleEndian.Uint32(b[16:]),
	}
	if magic := binary.LittleEndian.Uint32(b[20:]); magic != ^h.command {
		return adbHeader{}, fmt.Errorf("uxbridge: adb magic mismatch for command %#08x", h.command)
	}
	if h.length > adbMaxData {
		return adbHeader{}, fmt.Errorf("uxbridge: adb payload too large (%d bytes)", h.length)
	}
	return h, nil
}

// adbConn is a connected, authenticated-free adbd transport.
type adbConn struct {
	conn    net.Conn
	banner  string
	maxData uint32
	nextID  uint32
}

func adbDial(ctx context.Context, addr string, timeout time.Duration) (*adbConn, error) {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("uxbridge: dial adbd %s: %w", addr, err)
	}
	c := &adbConn{conn: conn, maxData: adbMaxData, nextID: 1}
	if err := c.handshake(timeout); err != nil {
		conn.Close()
		return nil, err
	}
	return c, nil
}

func (c *adbConn) handshake(timeout time.Duration) error {
	if err := c.write(encodeADBPacket(adbCNXN, adbVersion, adbMaxData, []byte(adbBanner)), timeout); err != nil {
		return err
	}
	h, data, err := c.read(timeout)
	if err != nil {
		return err
	}
	switch h.command {
	case adbCNXN:
		c.banner = strings.TrimRight(string(data), "\x00")
		if h.arg1 > 0 && h.arg1 < adbMaxData {
			c.maxData = h.arg1
		}
		return nil
	case adbAUTH:
		return errADBAuth
	default:
		return fmt.Errorf("uxbridge: unexpected adb reply %#08x to CNXN", h.command)
	}
}

func (c *adbConn) Close() error { return c.conn.Close() }

func (c *adbConn) write(p []byte, timeout time.Duration) error {
	if err := c.conn.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	if _, err := c.conn.Write(p); err != nil {
		return fmt.Errorf("uxbridge: adb write: %w", err)
	}
	return nil
}

func (c *adbConn) read(timeout time.Duration) (adbHeader, []byte, error) {
	if err := c.conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return adbHeader{}, nil, err
	}
	var raw [adbHeaderSize]byte
	if _, err := io.ReadFull(c.conn, raw[:]); err != nil {
		return adbHeader{}, nil, fmt.Errorf("uxbridge: adb read header: %w", err)
	}
	h, err := decodeADBHeader(raw[:])
	if err != nil {
		return adbHeader{}, nil, err
	}
	if h.length == 0 {
		return h, nil, nil
	}
	data := make([]byte, h.length)
	if _, err := io.ReadFull(c.conn, data); err != nil {
		return adbHeader{}, nil, fmt.Errorf("uxbridge: adb read payload: %w", err)
	}
	return h, data, nil
}

// Shell runs one command through the "shell:" service and returns everything
// adbd wrote before closing the stream. Output of dumpsys is large but bounded,
// so buffering it whole is fine.
func (c *adbConn) Shell(ctx context.Context, cmd string, timeout time.Duration) ([]byte, error) {
	local := c.nextID
	c.nextID++

	if err := c.write(encodeADBPacket(adbOPEN, local, 0, []byte("shell:"+cmd+"\x00")), timeout); err != nil {
		return nil, err
	}

	var (
		out    []byte
		remote uint32
		opened bool
	)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		h, data, err := c.read(timeout)
		if err != nil {
			return nil, err
		}
		switch h.command {
		case adbOKAY:
			if h.arg1 == local && !opened {
				remote, opened = h.arg0, true
			}
		case adbWRTE:
			if h.arg1 != local {
				continue
			}
			if !opened {
				remote, opened = h.arg0, true
			}
			out = append(out, data...)
			if err := c.write(encodeADBPacket(adbOKAY, local, remote, nil), timeout); err != nil {
				return nil, err
			}
		case adbCLSE:
			if h.arg1 != local {
				continue
			}
			if !opened {
				return nil, fmt.Errorf("uxbridge: adbd refused shell service for %q", cmd)
			}
			// Best effort: adbd does not care if the final CLSE is lost.
			_ = c.write(encodeADBPacket(adbCLSE, local, remote, nil), timeout)
			return out, nil
		case adbAUTH:
			return nil, errADBAuth
		}
	}
}
