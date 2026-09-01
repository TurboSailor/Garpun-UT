package garminhttp

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func gzipRxNetworks(t *testing.T, magic []byte, stamp int64) []byte {
	t.Helper()
	var payload bytes.Buffer
	payload.Write(magic)
	var ts [4]byte
	binary.BigEndian.PutUint32(ts[:], uint32(stamp))
	payload.Write(ts[:])
	payload.WriteString("ephemeris payload")

	var out bytes.Buffer
	zw := gzip.NewWriter(&out)
	if _, err := zw.Write(payload.Bytes()); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return out.Bytes()
}

func tarWith(t *testing.T, names ...string) []byte {
	t.Helper()
	var out bytes.Buffer
	tw := tar.NewWriter(&out)
	for _, name := range names {
		body := []byte("data for " + name)
		if err := tw.WriteHeader(&tar.Header{Name: name, Size: int64(len(body)), Mode: 0o644}); err != nil {
			t.Fatalf("tar header: %v", err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatalf("tar write: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	return out.Bytes()
}

func TestValidateAgpsRxNetworks(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	req := rawRequest(t, "https://api.gcs.garmin.com/ephemeris/rxnetworks/cpe.bin", nil)

	fresh := gzipRxNetworks(t, cpeRxNetworksMagic, now.Unix()-3600)
	if err := validateAgps(req, fresh, now); err != nil {
		t.Fatalf("fresh rxnetworks rejected: %v", err)
	}

	stale := gzipRxNetworks(t, cpeRxNetworksMagic, now.Unix()-agpsStaleAfter-1)
	if err := validateAgps(req, stale, now); err == nil {
		t.Fatal("stale rxnetworks accepted")
	}

	future := gzipRxNetworks(t, cpeRxNetworksMagic, now.Unix()+3600)
	if err := validateAgps(req, future, now); err == nil {
		t.Fatal("future rxnetworks accepted")
	}

	wrongMagic := gzipRxNetworks(t, []byte{0x02, 0x00}, now.Unix())
	if err := validateAgps(req, wrongMagic, now); err == nil {
		t.Fatal("wrong rxnetworks magic accepted")
	}

	if err := validateAgps(req, []byte("plain bytes"), now); err == nil {
		t.Fatal("non-gzip rxnetworks accepted")
	}
}

func TestValidateAgpsSonyCpe(t *testing.T) {
	now := time.Now()
	req := rawRequest(t, "https://api.gcs.garmin.com/ephemeris/cpe/sony/latest.bin", nil)

	good := append(append([]byte{}, cpeSonyMagic...), 0x11, 0x22)
	if err := validateAgps(req, good, now); err != nil {
		t.Fatalf("valid sony cpe rejected: %v", err)
	}
	if err := validateAgps(req, []byte{0x2a, 0x12, 0xa0, 0x03}, now); err == nil {
		t.Fatal("wrong sony magic accepted")
	}
}

func TestValidateAgpsTar(t *testing.T) {
	now := time.Now()
	req := rawRequest(t,
		"https://api.gcs.garmin.com/ephemeris/cpe/multi?constellations=GPS,GLONASS", nil)

	full := tarWith(t, "CPE_GPS.BIN", "CPE_GLO.BIN", "CPE_GAL.BIN")
	if err := validateAgps(req, full, now); err != nil {
		t.Fatalf("complete archive rejected: %v", err)
	}

	partial := tarWith(t, "CPE_GPS.BIN")
	if err := validateAgps(req, partial, now); err == nil {
		t.Fatal("archive missing GLONASS accepted")
	}

	if err := validateAgps(req, []byte("definitely not a tar"), now); err == nil {
		t.Fatal("non-tar accepted")
	}

	unknown := rawRequest(t,
		"https://api.gcs.garmin.com/ephemeris/cpe/multi?constellations=BEIDOU", nil)
	if err := validateAgps(unknown, full, now); err == nil {
		t.Fatal("unsupported constellation accepted")
	}
}

func TestValidateAgpsUnknownUrlShape(t *testing.T) {
	req := rawRequest(t, "https://api.gcs.garmin.com/ephemeris/something/else.bin", nil)
	if err := validateAgps(req, []byte{0x00}, time.Now()); err == nil {
		t.Fatal("unknown url shape accepted")
	}
}

func TestHandleAgpsServesAndCaches(t *testing.T) {
	now := time.Now()
	blob := gzipRxNetworks(t, cpeRxNetworksMagic, now.Unix()-60)
	sum := md5.Sum(blob)
	etag := `"` + hex.EncodeToString(sum[:]) + `"`

	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.Write(blob)
	}))
	defer srv.Close()

	h := testHandler(t, Options{
		AgpsURL: func(string) string { return srv.URL },
	})

	req := rawRequest(t,
		"https://api.gcs.garmin.com/ephemeris/rxnetworks/cpe.bin",
		map[string]string{"Accept": "application/octet-stream"},
	)
	resp := h.handleAgps(req)
	if resp == nil {
		t.Fatal("expected agps data")
	}
	if resp.Status != 200 {
		t.Fatalf("status = %d", resp.Status)
	}
	if !bytes.Equal(resp.Body, blob) {
		t.Fatal("body does not match the fetched blob")
	}
	if resp.Header("etag") != etag {
		t.Fatalf("etag = %q, want %q", resp.Header("etag"), etag)
	}
	if resp.Header("cache-control") != "max-age=14400" {
		t.Fatalf("cache-control = %q", resp.Header("cache-control"))
	}
	if resp.Header("content-type") != "application/octet-stream" {
		t.Fatalf("content-type = %q", resp.Header("content-type"))
	}

	// A matching if-none-match yields an empty 304 and no second fetch.
	cached := rawRequest(t,
		"https://api.gcs.garmin.com/ephemeris/rxnetworks/cpe.bin",
		map[string]string{"If-None-Match": etag},
	)
	resp = h.handleAgps(cached)
	if resp == nil || resp.Status != 304 {
		t.Fatalf("expected 304, got %+v", resp)
	}
	if len(resp.Body) != 0 {
		t.Fatalf("304 must have an empty body, got %d bytes", len(resp.Body))
	}
	if hits != 1 {
		t.Fatalf("upstream fetched %d times, want 1", hits)
	}
}

func TestHandleAgpsRefusesWithoutSource(t *testing.T) {
	h := testHandler(t, Options{AgpsURL: func(string) string { return "" }})
	req := rawRequest(t, "https://api.gcs.garmin.com/ephemeris/rxnetworks/cpe.bin", nil)
	if resp := h.handleAgps(req); resp != nil {
		t.Fatalf("expected a refusal, got %+v", resp)
	}
}
