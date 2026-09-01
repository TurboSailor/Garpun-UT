package uxbridge

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"net"
	"testing"
	"time"
)

func TestEncodeDecodeADBPacket(t *testing.T) {
	payload := []byte("shell:dumpsys notification\x00")
	pkt := encodeADBPacket(adbOPEN, 7, 0, payload)

	if len(pkt) != adbHeaderSize+len(payload) {
		t.Fatalf("packet is %d bytes, want %d", len(pkt), adbHeaderSize+len(payload))
	}
	h, err := decodeADBHeader(pkt)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if h.command != adbOPEN || h.arg0 != 7 || h.arg1 != 0 {
		t.Errorf("header = %+v", h)
	}
	if int(h.length) != len(payload) {
		t.Errorf("length = %d, want %d", h.length, len(payload))
	}
	if h.checksum != adbChecksum(payload) {
		t.Errorf("checksum = %d", h.checksum)
	}
	if !bytes.Equal(pkt[adbHeaderSize:], payload) {
		t.Error("payload corrupted")
	}
}

func TestADBChecksumIsByteSum(t *testing.T) {
	if got := adbChecksum([]byte{0xff, 0x01, 0x02}); got != 0x102 {
		t.Errorf("checksum = %#x, want 0x102", got)
	}
	if got := adbChecksum(nil); got != 0 {
		t.Errorf("checksum(nil) = %d", got)
	}
}

func TestDecodeADBHeaderRejectsBadMagic(t *testing.T) {
	pkt := encodeADBPacket(adbCNXN, 0, 0, nil)
	binary.LittleEndian.PutUint32(pkt[20:], 0xdeadbeef)
	if _, err := decodeADBHeader(pkt); err == nil {
		t.Error("bad magic accepted")
	}
	if _, err := decodeADBHeader(pkt[:10]); err == nil {
		t.Error("short header accepted")
	}

	oversized := encodeADBPacket(adbWRTE, 1, 1, nil)
	binary.LittleEndian.PutUint32(oversized[12:], adbMaxData+1)
	if _, err := decodeADBHeader(oversized); err == nil {
		t.Error("oversized payload accepted")
	}
}

// fakeADBD speaks just enough of the protocol to exercise the client: it
// answers CNXN, accepts one stream and streams a canned reply in two chunks.
func fakeADBD(t *testing.T, auth bool, reply string) (addr string, gotCmd chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	gotCmd = make(chan string, 1)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		read := func() (adbHeader, []byte) {
			var raw [adbHeaderSize]byte
			if _, err := readFull(conn, raw[:]); err != nil {
				return adbHeader{}, nil
			}
			h, err := decodeADBHeader(raw[:])
			if err != nil {
				return adbHeader{}, nil
			}
			data := make([]byte, h.length)
			if h.length > 0 {
				if _, err := readFull(conn, data); err != nil {
					return adbHeader{}, nil
				}
			}
			return h, data
		}

		if h, _ := read(); h.command != adbCNXN {
			return
		}
		if auth {
			conn.Write(encodeADBPacket(adbAUTH, 1, 0, []byte("token")))
			return
		}
		conn.Write(encodeADBPacket(adbCNXN, adbVersion, adbMaxData, []byte("device::ro.product.name=waydroid")))

		h, data := read()
		if h.command != adbOPEN {
			return
		}
		gotCmd <- string(bytes.TrimRight(data, "\x00"))
		local := h.arg0
		const remote = 42
		conn.Write(encodeADBPacket(adbOKAY, remote, local, nil))

		half := len(reply) / 2
		for _, chunk := range []string{reply[:half], reply[half:]} {
			conn.Write(encodeADBPacket(adbWRTE, remote, local, []byte(chunk)))
			if h, _ := read(); h.command != adbOKAY {
				return
			}
		}
		conn.Write(encodeADBPacket(adbCLSE, remote, local, nil))
	}()
	return ln.Addr().String(), gotCmd
}

func readFull(c net.Conn, p []byte) (int, error) {
	n := 0
	for n < len(p) {
		c.SetReadDeadline(time.Now().Add(5 * time.Second))
		m, err := c.Read(p[n:])
		if err != nil {
			return n, err
		}
		n += m
	}
	return n, nil
}

func TestADBShellRoundTrip(t *testing.T) {
	const reply = "Current Notification Manager state:\n  Notification List:\n"
	addr, gotCmd := fakeADBD(t, false, reply)

	c, err := adbDial(context.Background(), addr, 5*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if c.banner != "device::ro.product.name=waydroid" {
		t.Errorf("banner = %q", c.banner)
	}

	out, err := c.Shell(context.Background(), dumpsysCommand, 5*time.Second)
	if err != nil {
		t.Fatalf("shell: %v", err)
	}
	if string(out) != reply {
		t.Errorf("output = %q, want %q", out, reply)
	}
	if cmd := <-gotCmd; cmd != "shell:"+dumpsysCommand {
		t.Errorf("service = %q", cmd)
	}
}

func TestADBDialReportsAuth(t *testing.T) {
	addr, _ := fakeADBD(t, true, "")
	_, err := adbDial(context.Background(), addr, 5*time.Second)
	if !errors.Is(err, errADBAuth) {
		t.Fatalf("err = %v, want errADBAuth", err)
	}
}
