package garminhttp

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Ephemeris (AGPS) blobs. The watch asks Garmin for them through the proxy; we
// fetch them from a configurable mirror, sanity check the format and serve them
// with an etag so the watch can skip unchanged data.

const (
	agpsMaxBytes   = 1 << 20 // they are usually ~60 KB
	agpsMaxAge     = 14400   // seconds, matches the cache-control we advertise
	agpsStaleAfter = 604800  // rxnetworks payloads older than 7 days are stale
)

var (
	gzHeader           = []byte{0x1f, 0x8b}
	cpeRxNetworksMagic = []byte{0x01, 0x00}
	cpeSonyMagic       = []byte{0x2a, 0x12, 0xa0, 0x02}
)

// agpsConstellationFiles maps the constellations query values onto the file
// names that must be present in the tar archive.
var agpsConstellationFiles = map[string]string{
	"GPS":     "CPE_GPS.BIN",
	"GLONASS": "CPE_GLO.BIN",
	"GALILEO": "CPE_GAL.BIN",
	"QZSS":    "CPE_QZSS.BIN",
}

type agpsEntry struct {
	data    []byte
	etag    string
	fetched time.Time
}

func isAgpsRequest(req *Request) bool {
	return req.Host() == "api.gcs.garmin.com" && strings.HasPrefix(req.Path(), "/ephemeris/")
}

func (h *Handler) handleAgps(req *Request) *Response {
	source := req.RawURL
	if h.opts.AgpsURL != nil {
		source = h.opts.AgpsURL(req.RawURL)
	}
	if source == "" {
		h.log.Warn("garminhttp: no agps source for url", "url", req.RawURL)
		return nil
	}

	entry, err := h.agpsFetch(source)
	if err != nil {
		h.log.Warn("garminhttp: unable to obtain agps data", "url", source, "err", err)
		return nil
	}

	resp := newResponse(200)
	resp.SetHeader("etag", entry.etag)

	if match, ok := req.Headers["if-none-match"]; ok && match == entry.etag {
		h.log.Debug("garminhttp: agps unchanged", "etag", entry.etag)
		resp.Status = 304
		resp.Body = nil
		return resp
	}
	resp.SetHeader("cache-control", fmt.Sprintf("max-age=%d", agpsMaxAge))

	if err := validateAgps(req, entry.data, time.Now()); err != nil {
		h.log.Warn("garminhttp: refusing agps data", "url", source, "err", err)
		return nil
	}

	contentType := req.Header("accept")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	resp.SetHeader("Content-Type", contentType)
	resp.Body = entry.data
	resp.onSent = func() {
		h.log.Info("garminhttp: agps data sent to watch", "bytes", len(entry.data))
	}
	h.log.Info("garminhttp: sending agps data", "url", source, "bytes", len(entry.data))
	return resp
}

// agpsFetch downloads and caches the blob for the lifetime of its cache-control
// window, so repeated watch requests do not hit the network again.
func (h *Handler) agpsFetch(source string) (*agpsEntry, error) {
	h.mu.Lock()
	entry, ok := h.agps[source]
	h.mu.Unlock()
	if ok && time.Since(entry.fetched) < agpsMaxAge*time.Second {
		return entry, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.opts.Timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, fmt.Errorf("garminhttp: agps request: %w", err)
	}
	httpResp, err := h.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("garminhttp: agps fetch: %w", err)
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("garminhttp: agps fetch: http %d", httpResp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(httpResp.Body, agpsMaxBytes))
	if err != nil {
		return nil, fmt.Errorf("garminhttp: agps read: %w", err)
	}

	sum := md5.Sum(data)
	entry = &agpsEntry{
		data:    data,
		etag:    `"` + hex.EncodeToString(sum[:]) + `"`,
		fetched: time.Now(),
	}
	h.mu.Lock()
	h.agps[source] = entry
	h.mu.Unlock()
	return entry, nil
}

// validateAgps runs the format checks for the url shape the watch used.
func validateAgps(req *Request, data []byte, now time.Time) error {
	if constellations := req.Query.Get("constellations"); constellations != "" {
		return validateAgpsTar(data, strings.Split(constellations, ","))
	}
	if strings.Contains(req.Path(), "/rxnetworks/") {
		return validateAgpsRxNetworks(data, now)
	}
	if strings.HasPrefix(req.Path(), "/ephemeris/cpe/sony") {
		if !bytes.HasPrefix(data, cpeSonyMagic) {
			return errors.New("sony cpe header mismatch")
		}
		return nil
	}
	return errors.New("unknown agps url shape")
}

func validateAgpsTar(data []byte, constellations []string) error {
	names := make(map[string]struct{})
	tr := tar.NewReader(bytes.NewReader(data))
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("not a tar archive: %w", err)
		}
		names[hdr.Name] = struct{}{}
	}
	if len(names) == 0 {
		return errors.New("empty tar archive")
	}
	for _, c := range constellations {
		file, ok := agpsConstellationFiles[strings.TrimSpace(c)]
		if !ok {
			return fmt.Errorf("unsupported constellation %q", c)
		}
		if _, ok := names[file]; !ok {
			return fmt.Errorf("archive is missing %s", file)
		}
	}
	return nil
}

func validateAgpsRxNetworks(data []byte, now time.Time) error {
	if !bytes.HasPrefix(data, gzHeader) {
		return errors.New("not a gzip stream")
	}
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer zr.Close()

	var head [6]byte
	if _, err := io.ReadFull(zr, head[:]); err != nil {
		return fmt.Errorf("read rxnetworks header: %w", err)
	}
	if !bytes.Equal(head[:2], cpeRxNetworksMagic) {
		return fmt.Errorf("rxnetworks header mismatch %x", head[:2])
	}
	stamp := int64(binary.BigEndian.Uint32(head[2:]))
	age := now.Unix() - stamp
	if age < 0 {
		return fmt.Errorf("rxnetworks timestamp %d is in the future", stamp)
	}
	if age > agpsStaleAfter {
		return fmt.Errorf("rxnetworks timestamp %d is older than 7 days", stamp)
	}
	return nil
}
