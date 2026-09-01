package gfdi

import (
	"bytes"
	"encoding/hex"
	"math/rand/v2"
	"testing"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	return b
}

// Vectors captured from real watches and preserved in the upstream
// CobsCoDecTest, so the Go codec has to reproduce them byte for byte.
var cobsVectors = []struct {
	name  string
	frame string
	cobs  string
}{
	{
		name:  "instinct device information",
		frame: "2C00A0139600310F684C1BCA840508020B496E7374696E6374203253084" + "96E7374696E637402325300000" + "4B8",
		cobs:  "00022C04A0139623310F684C1BCA840508020B496E7374696E637420325308496E7374696E6374023253010304B800",
	},
	{
		name:  "device information with padding",
		frame: "2b008813a013009600ffffffffffffa71fffff046c61726a07756e6b6e6f776e0758512d4343373201f9cf",
		cobs:  "00022b058813a013029623ffffffffffffa71fffff046c61726a07756e6b6e6f776e0758512d4343373201f9cf00",
	},
	{
		name:  "protobuf response status",
		frame: "11008813B41300370300000000000037E4",
		cobs:  "000211058813B41303370301010101010337E400",
	},
}

func TestEncodeCOBSMatchesCapturedFrames(t *testing.T) {
	for _, v := range cobsVectors {
		t.Run(v.name, func(t *testing.T) {
			frame := mustHex(t, v.frame)
			want := mustHex(t, v.cobs)
			got := EncodeCOBS(frame)
			if !bytes.Equal(got, want) {
				t.Errorf("encode mismatch\n got %X\nwant %X", got, want)
			}
		})
	}
}

func TestDecodeCOBSMatchesCapturedFrames(t *testing.T) {
	for _, v := range cobsVectors {
		t.Run(v.name, func(t *testing.T) {
			d := NewCobsDecoder()
			frames := d.Feed(mustHex(t, v.cobs))
			if len(frames) != 1 {
				t.Fatalf("got %d frames, want 1", len(frames))
			}
			if want := mustHex(t, v.frame); !bytes.Equal(frames[0], want) {
				t.Errorf("decode mismatch\n got %X\nwant %X", frames[0], want)
			}
		})
	}
}

// A watch splits one COBS frame across several BLE notifications; the decoder
// must reassemble regardless of where the cuts land.
func TestDecodeAcrossNotifications(t *testing.T) {
	v := cobsVectors[0]
	full := mustHex(t, v.cobs)
	want := mustHex(t, v.frame)

	for _, chunk := range []int{1, 3, 19, 20, 47} {
		d := NewCobsDecoder()
		var got [][]byte
		for off := 0; off < len(full); off += chunk {
			end := off + chunk
			if end > len(full) {
				end = len(full)
			}
			got = append(got, d.Feed(full[off:end])...)
		}
		if len(got) != 1 {
			t.Fatalf("chunk %d: got %d frames, want 1", chunk, len(got))
		}
		if !bytes.Equal(got[0], want) {
			t.Fatalf("chunk %d: payload mismatch", chunk)
		}
	}
}

// A dropped notification must not wedge the decoder: the damaged frame is
// discarded and the next intact one still decodes.
func TestTruncatedFrameDoesNotWedgeDecoder(t *testing.T) {
	good := mustHex(t, cobsVectors[0].cobs)
	want := mustHex(t, cobsVectors[0].frame)

	d := NewCobsDecoder()
	// Feed a frame with its middle missing, then a whole one.
	broken := append(append([]byte{}, good[:10]...), good[30:]...)
	d.Feed(broken)
	frames := d.Feed(good)

	found := false
	for _, f := range frames {
		if bytes.Equal(f, want) {
			found = true
		}
	}
	if !found {
		t.Fatalf("decoder did not recover; got %d frames", len(frames))
	}
}

func TestOversizedStreamResetsDecoder(t *testing.T) {
	d := NewCobsDecoder()
	// No terminator at all: the buffer must not grow past the 10000 byte cap.
	junk := bytes.Repeat([]byte{0x01}, cobsBufferSize+500)
	d.Feed(junk)

	good := mustHex(t, cobsVectors[1].cobs)
	want := mustHex(t, cobsVectors[1].frame)
	frames := d.Feed(good)
	if len(frames) != 1 || !bytes.Equal(frames[0], want) {
		t.Fatalf("decoder did not recover after overflow: %d frames", len(frames))
	}
}

func TestCOBSRoundTripFuzz(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for range 500 {
		n := 1 + rng.IntN(2000)
		payload := make([]byte, n)
		for i := range payload {
			// Bias towards zero bytes: they are what COBS has to escape.
			if rng.IntN(3) == 0 {
				payload[i] = 0
			} else {
				payload[i] = byte(rng.IntN(256))
			}
		}
		encoded := EncodeCOBS(payload)
		d := NewCobsDecoder()
		var got [][]byte
		mtu := 19 + rng.IntN(500)
		for off := 0; off < len(encoded); off += mtu {
			end := off + mtu
			if end > len(encoded) {
				end = len(encoded)
			}
			got = append(got, d.Feed(encoded[off:end])...)
		}
		if len(got) != 1 || !bytes.Equal(got[0], payload) {
			t.Fatalf("round trip failed for %d bytes (mtu %d): got %d frames", n, mtu, len(got))
		}
	}
}

// CRC vectors verified against the bytes the watch actually sent.
func TestCRC16(t *testing.T) {
	cases := []struct {
		data string
		want uint16
	}{
		{"2C00A0139600310F684C1BCA840508020B496E7374696E637420325308496E7374696E63740232530000", 0xB804},
		{"11008813B413003703000000000000", 0xE437},
	}
	for _, c := range cases {
		if got := CRC16(0, mustHex(t, c.data)); got != c.want {
			t.Errorf("CRC16(%s) = %04X, want %04X", c.data[:16], got, c.want)
		}
	}
}

func TestBuildAndParseFrame(t *testing.T) {
	payload := []byte{0x01, 0x02, 0x03, 0x00, 0xFF}
	frame := BuildFrame(MsgSystemEvent, payload)

	parsed, err := ParseFrame(frame)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.Type != MsgSystemEvent {
		t.Errorf("type = %d, want %d", parsed.Type, MsgSystemEvent)
	}
	if !bytes.Equal(parsed.Payload, payload) {
		t.Errorf("payload = %X, want %X", parsed.Payload, payload)
	}

	// A single flipped bit has to fail the checksum.
	corrupt := append([]byte{}, frame...)
	corrupt[5] ^= 0x01
	if _, err := ParseFrame(corrupt); err == nil {
		t.Error("expected a CRC error on a corrupted frame")
	}
}

// Watches abbreviate frequent message types: bit 15 set means "low byte plus
// 5000", with a sequence number in between that upstream ignores.
func TestParseFrameShortMessageType(t *testing.T) {
	frame := mustHex(t, "11008813B41300370300000000000037E4")
	parsed, err := ParseFrame(frame)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.Type != MsgResponse {
		t.Fatalf("type = %d, want %d", parsed.Type, MsgResponse)
	}
	st, err := ParseStatus(parsed.Payload)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.OriginalType != MsgProtobufResponse {
		t.Errorf("original type = %d, want %d", st.OriginalType, MsgProtobufResponse)
	}
	if !st.OK() {
		t.Errorf("status = %d, want ACK", st.Status)
	}
}

func TestDeviceInformationRoundTrip(t *testing.T) {
	// The exact frame an Instinct 2 sends on connect.
	frame := mustHex(t, cobsVectors[0].frame)
	parsed, err := ParseFrame(frame)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.Type != MsgDeviceInformation {
		t.Fatalf("type = %d, want %d", parsed.Type, MsgDeviceInformation)
	}
	info, err := ParseDeviceInformation(parsed.Payload)
	if err != nil {
		t.Fatalf("device information: %v", err)
	}
	if info.BluetoothName != "Instinct 2S" {
		t.Errorf("bluetooth name = %q", info.BluetoothName)
	}
	if info.DeviceName != "Instinct" || info.DeviceModel != "2S" {
		t.Errorf("name/model = %q/%q", info.DeviceName, info.DeviceModel)
	}
	if info.MaxPacketSize == 0 {
		t.Error("max packet size not parsed")
	}
}

func TestOurCapabilitiesBitfield(t *testing.T) {
	caps := OurCapabilities()
	if len(caps) != 15 {
		t.Fatalf("capability bitfield = %d bytes, want 15", len(caps))
	}
	set := DecodeCapabilities(caps)
	for _, want := range []int{CapSync, CapFindMyPhone, CapFindMyWatch, CapWeatherConditions,
		CapMultiLinkService, CapRealtimeSettings, 103, 112, 113} {
		if !set[want] {
			t.Errorf("capability %d not advertised", want)
		}
	}
	// Garmin Connect dumps never set these, so neither do we.
	for _, unwanted := range []int{104, 108, 111, 114, 119} {
		if set[unwanted] {
			t.Errorf("capability %d should not be advertised", unwanted)
		}
	}
}
