package garminhttp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
)

// Everything the local interceptors did not claim ends up here: Garmin's own
// domains are blocked outright, the rest is proxied to the internet.

// blockedSuffixes are refused even if the user would allow them: with fake
// OAuth credentials the requests are useless, and they would leak the fake
// tokens to Garmin.
var blockedSuffixes = []string{"garmin.com", "dciwx.com"}

func isBlockedHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, suffix := range blockedSuffixes {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}

func (h *Handler) handleFirewall(req *Request) *Response {
	if isBlockedHost(req.Host()) {
		h.log.Warn("garminhttp: blocking request to garmin domain", "host", req.Host())
		return nil
	}

	// The network round trip must not block the GFDI dispatcher.
	go h.proxy(req)

	return &Response{complete: false}
}

func (h *Handler) proxy(req *Request) {
	resp, err := h.fetch(req)
	if err != nil {
		h.log.Warn("garminhttp: proxied request failed", "url", req.RawURL, "err", err)
		h.sendAsync(req, nil)
		return
	}
	h.log.Debug("garminhttp: proxied request done",
		"url", req.RawURL, "status", resp.Status, "bytes", len(resp.Body))
	h.sendAsync(req, resp)
}

func (h *Handler) fetch(req *Request) (*Response, error) {
	method := req.Method
	if _, ok := knownMethods[method]; !ok {
		method = http.MethodGet
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.opts.Timeout)
	defer cancel()

	var body io.Reader
	if len(req.Body) > 0 {
		body = bytes.NewReader(req.Body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, req.RawURL, body)
	if err != nil {
		return nil, err
	}
	for k, v := range req.Headers {
		switch k {
		case "accept-encoding", "host", "content-length", "connection":
			// Transport-level headers: let net/http negotiate them, we
			// re-compress on the way back to the watch if it asked for gzip.
			continue
		}
		httpReq.Header.Set(k, v)
	}

	httpResp, err := h.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	limit := int64(h.opts.MaxResponseBytes)
	data, err := io.ReadAll(io.LimitReader(httpResp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("garminhttp: response exceeds size limit")
	}

	resp := newResponse(httpResp.StatusCode)
	resp.Body = data
	for _, key := range []string{"content-type", "etag", "cache-control", "last-modified", "expires"} {
		if v := httpResp.Header.Get(key); v != "" {
			resp.SetHeader(key, v)
		}
	}
	return resp, nil
}

var knownMethods = map[string]struct{}{
	http.MethodGet:    {},
	http.MethodPut:    {},
	http.MethodPost:   {},
	http.MethodDelete: {},
	http.MethodPatch:  {},
	http.MethodHead:   {},
}
