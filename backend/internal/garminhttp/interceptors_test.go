package garminhttp

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	pb "pulse/backend/internal/gproto/garmin"
)

func testHandler(t *testing.T, opts Options) *Handler {
	t.Helper()
	return New(slog.New(slog.NewTextHandler(io.Discard, nil)), opts)
}

func rawRequest(t *testing.T, url string, headers map[string]string) *Request {
	t.Helper()
	method := pb.HttpService_GET
	raw := &pb.HttpService_RawRequest{Url: &url, Method: &method}
	for k, v := range headers {
		key, value := k, v
		raw.Header = append(raw.Header, &pb.HttpService_Header{Key: &key, Value: &value})
	}
	req, err := newRawRequest(1, raw)
	if err != nil {
		t.Fatalf("newRawRequest: %v", err)
	}
	return req
}

func TestIsBlockedHost(t *testing.T) {
	blocked := []string{
		"garmin.com",
		"api.gcs.garmin.com",
		"connectapi.garmin.com",
		"GARMIN.COM",
		"cache.dciwx.com",
		"dciwx.com",
		"garmin.com.",
	}
	for _, host := range blocked {
		if !isBlockedHost(host) {
			t.Errorf("%s should be blocked", host)
		}
	}
	allowed := []string{
		"example.com",
		"api.open-meteo.com",
		// A suffix match must not fire on a different registrable domain.
		"notgarmin.com",
		"mygarmin.com",
		"garmin.com.evil.net",
		"",
	}
	for _, host := range allowed {
		if isBlockedHost(host) {
			t.Errorf("%s should not be blocked", host)
		}
	}
}

func TestFirewallBlocksGarminDomains(t *testing.T) {
	h := testHandler(t, Options{})
	req := rawRequest(t, "https://connectapi.garmin.com/some/path", nil)
	if resp := h.handleFirewall(req); resp != nil {
		t.Fatalf("expected the garmin request to be refused, got %+v", resp)
	}
}

func TestInterceptorRoutingOrder(t *testing.T) {
	h := testHandler(t, Options{})
	tests := []struct {
		url     string
		headers map[string]string
		want    string
	}{
		{"https://api.gcs.garmin.com/weather/v2/current?lat=1&lon=2", nil, "weather"},
		{"https://cache.dciwx.com/weather/v1/forecast/day", nil, "weather"},
		{"https://api.gcs.garmin.com/ephemeris/cpe/sony/x.bin", nil, "agps"},
		{"https://connectapi.garmin.com/api/oauth/token", nil, "oauth"},
		{"https://connectapi.garmin.com/device-gateway/usercontact/contacts", nil, "contacts"},
		{"https://api.gcs.garmin.com/image-service/icon/1", nil, "firewall"},
		{"https://example.com/anything", nil, "firewall"},
	}
	for _, tt := range tests {
		req := rawRequest(t, tt.url, tt.headers)
		var got string
		for _, in := range h.chain {
			if in.matches(req) {
				got = in.name
				break
			}
		}
		if got != tt.want {
			t.Errorf("%s routed to %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestWeatherWithoutSourceReplies404(t *testing.T) {
	h := testHandler(t, Options{})
	req := rawRequest(t, "https://api.gcs.garmin.com/weather/v2/current?lat=1&lon=2", nil)
	resp := h.handleWeather(req)
	if resp == nil {
		t.Fatal("expected a response")
	}
	if resp.Status != 404 {
		t.Fatalf("status = %d, want 404", resp.Status)
	}
}

func TestOauthRefreshKeepsToken(t *testing.T) {
	h := testHandler(t, Options{})
	url := "https://connectapi.garmin.com/api/oauth/token"
	method := pb.HttpService_POST
	raw := &pb.HttpService_RawRequest{
		Url:     &url,
		Method:  &method,
		RawBody: []byte("grant_type=refresh_token&refresh_token=abc-123&client_id=x"),
	}
	req, err := newRawRequest(7, raw)
	if err != nil {
		t.Fatalf("newRawRequest: %v", err)
	}
	resp := h.handleOauth(req)
	if resp == nil || resp.Status != 200 {
		t.Fatalf("unexpected response %+v", resp)
	}
	if resp.Header("content-type") != "application/json" {
		t.Fatalf("content type = %q", resp.Header("content-type"))
	}
	if !strings.Contains(string(resp.Body), `"refresh_token":"abc-123"`) {
		t.Fatalf("refresh token not echoed back: %s", resp.Body)
	}
}

func TestOauthRequiresPost(t *testing.T) {
	h := testHandler(t, Options{})
	req := rawRequest(t, "https://connectapi.garmin.com/api/oauth/token", nil)
	if resp := h.handleOauth(req); resp != nil {
		t.Fatalf("GET should be refused, got %+v", resp)
	}
}

func TestContactsRepliesEmptyBook(t *testing.T) {
	h := testHandler(t, Options{})
	req := rawRequest(t,
		"https://connectapi.garmin.com/device-gateway/usercontact/contacts",
		map[string]string{"Accept": "application/octet-stream"},
	)
	resp := h.handleContacts(req)
	if resp == nil || resp.Status != 200 {
		t.Fatalf("unexpected response %+v", resp)
	}
	var decoded pb.Response
	if err := proto.Unmarshal(resp.Body, &decoded); err != nil {
		t.Fatalf("unmarshal contacts: %v", err)
	}
	if len(decoded.GetContact()) != 0 {
		t.Fatalf("expected no contacts, got %d", len(decoded.GetContact()))
	}
	if decoded.GetSelf().GetId() != "SELF" {
		t.Fatalf("self id = %q", decoded.GetSelf().GetId())
	}
}

func TestContactsRequiresProtobufAccept(t *testing.T) {
	h := testHandler(t, Options{})
	req := rawRequest(t,
		"https://connectapi.garmin.com/device-gateway/usercontact/contacts",
		map[string]string{"Accept": "application/json"},
	)
	if resp := h.handleContacts(req); resp != nil {
		t.Fatalf("json accept should be refused, got %+v", resp)
	}
}
