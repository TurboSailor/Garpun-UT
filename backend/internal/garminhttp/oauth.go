package garminhttp

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	pb "pulse/backend/internal/gproto/garmin"
)

// Fake OAuth: the watch insists on being logged in to Garmin Connect, but never
// verifies the tokens locally. Handing out random ones keeps it happy, and the
// firewall makes sure they never reach a real Garmin server.

// oauthScopes is the union of the scope lists observed on Swim 2, Venu 3 and
// Enduro 3.
var oauthScopes = []string{
	"GCS_EPHEMERIS_SONY_READ",
	"GCS_CIQ_APPSTORE_MOBILE_READ",
	"GCS_EMERGENCY_ASSISTANCE_CREATE",
	"GCS_GEOLOCATION_ELEVATION_READ",
	"GCS_IMAGE_READ",
	"GCS_LIVETRACK_FIT_CREATE",
	"GCS_LIVETRACK_FIT_READ",
	"GCS_LIVETRACK_FIT_UPDATE",
	"OMT_GOLF_SUBSCRIPTION_READ",
	"OMT_SUBSCRIPTION_READ",
	"CSE_CDS_ACCOUNT_READ",
	"GCS_DEVICE_INSTRUCTION_OUTDOOR_READ",
	"GCS_IMAGE_STORAGE_READ",
	"GCS_LIVE_EVENT_SHARING_CREATE",
	"GCS_LTE_SIGNAL_UPDATE",
	"GCS_MESSAGING_FITNESS_CREATE",
	"GCS_MESSAGING_FITNESS_READ",
	"GCS_MESSAGING_FITNESS_UPDATE",
	"GCS_STOCKS_READ",
	"GCS_TIDE_READ",
	"GCS_WEATHER_RACEDAY_READ",
	"MARINE_SERVER_ACCESS",
	"MARINE_SERVER_CHARTS_SUBSCRIPTION_READ",
	"MARINE_SERVER_CHARTS_SUBSCRIPTION_WRITE",
	"OMT_BIRDSEYE_READ",
	"OMT_OUTDOOR_MAP_SUBSCRIPTION_CREATE",
	"OMT_OUTDOOR_MAP_SUBSCRIPTION_DELETE",
	"OMT_OUTDOOR_MAP_SUBSCRIPTION_READ",
	"YAR_BILLING_SUBSCRIBER_READ",
	"YAR_INREACH_HERMES_READ",
	"YAR_INREACH_IRIS_CREATE",
	"YAR_INREACH_IRIS_READ",
	"YAR_INREACH_IRIS_UPDATE",
	"YAR_INREACH_VOICE_EVENT_CREATE",
}

// authorizationResponse is the camelCase flavour used by the exchange endpoints.
type authorizationResponse struct {
	AccessToken           string `json:"accessToken"`
	TokenType             string `json:"tokenType"`
	RefreshToken          string `json:"refreshToken"`
	ExpiresIn             int    `json:"expiresIn"`
	Scope                 string `json:"scope"`
	RefreshTokenExpiresIn string `json:"refreshTokenExpiresIn"`
	CustomerID            string `json:"customerId"`
}

// refreshResponse is the snake_case flavour used by the token endpoints.
type refreshResponse struct {
	AccessToken           string `json:"access_token"`
	TokenType             string `json:"token_type"`
	ExpiresIn             int    `json:"expires_in"`
	Scope                 string `json:"scope"`
	RefreshToken          string `json:"refresh_token"`
	RefreshTokenExpiresIn string `json:"refresh_token_expires_in"`
	CustomerID            string `json:"customerId"`
}

func isOauthRequest(req *Request) bool {
	path := req.Path()
	return strings.HasPrefix(path, "/api/oauth/") ||
		strings.HasPrefix(path, "/oauth/") ||
		strings.HasPrefix(path, "/oauthTokenExchangeService/")
}

func (h *Handler) handleOauth(req *Request) *Response {
	if req.Method != "POST" {
		h.log.Warn("garminhttp: oauth request is not a POST", "method", req.Method, "path", req.Path())
		return nil
	}
	scope := strings.Join(oauthScopes, " ")

	var payload any
	switch req.Path() {
	case "/oauthTokenExchangeService/connectToIT", "/oauth/connect_exchange/token":
		payload = authorizationResponse{
			AccessToken:           randomUUID(),
			TokenType:             "Bearer",
			RefreshToken:          randomUUID(),
			ExpiresIn:             7776000,
			Scope:                 scope,
			RefreshTokenExpiresIn: "31536000",
			CustomerID:            randomUUID(),
		}
	case "/api/oauth/token", "/oauth/refresh_token/token":
		// Keep the refresh token the watch sent us, if any.
		refresh := formValue(string(req.Body), "refresh_token")
		if refresh == "" {
			h.log.Warn("garminhttp: oauth refresh without refresh_token")
			refresh = randomUUID()
		}
		payload = refreshResponse{
			AccessToken:           randomUUID(),
			TokenType:             "Bearer",
			ExpiresIn:             7776000,
			Scope:                 scope,
			RefreshToken:          refresh,
			RefreshTokenExpiresIn: "31536000",
			CustomerID:            randomUUID(),
		}
	default:
		h.log.Warn("garminhttp: unknown oauth path", "path", req.Path())
		return nil
	}

	body, err := json.Marshal(payload)
	if err != nil {
		h.log.Error("garminhttp: marshal oauth response", "err", err)
		return nil
	}
	resp := newResponse(200)
	resp.Body = body
	resp.SetHeader("Content-Type", "application/json")
	return resp
}

func isContactsRequest(req *Request) bool {
	return req.Host() == "connectapi.garmin.com" &&
		strings.HasPrefix(req.Path(), "/device-gateway/usercontact/")
}

// handleContacts answers with an empty contact book. Pulse has no contact
// source on Ubuntu Touch, but the watch needs a well-formed reply.
func (h *Handler) handleContacts(req *Request) *Response {
	if req.Path() != "/device-gateway/usercontact/contacts" {
		h.log.Warn("garminhttp: unknown contacts path", "path", req.Path())
		return nil
	}
	if req.Header("accept") != "application/octet-stream" {
		h.log.Warn("garminhttp: unsupported contacts content type", "accept", req.Header("accept"))
		return nil
	}

	selfID := "SELF"
	empty := ""
	zero := uint32(0)
	now := uint64(time.Now().UnixMilli())
	contacts := &pb.Response{
		Self: &pb.Contact{
			Id:         &selfID,
			FullName:   &empty,
			FirstName:  &empty,
			LastName:   &empty,
			Unk7:       &empty,
			Unk8:       &zero,
			Unk9:       &zero,
			Unk10:      &zero,
			Unk12:      &zero,
			Unk21:      &zero,
			UpdateTime: &now,
		},
	}
	body, err := proto.Marshal(contacts)
	if err != nil {
		h.log.Error("garminhttp: marshal contacts", "err", err)
		return nil
	}
	resp := newResponse(200)
	resp.Body = body
	resp.SetHeader("Content-Type", "application/octet-stream")
	return resp
}

// formValue pulls one key out of an application/x-www-form-urlencoded body
// without percent-decoding, matching what the watch expects back verbatim.
func formValue(body, key string) string {
	for _, pair := range strings.Split(body, "&") {
		name, value, ok := strings.Cut(pair, "=")
		if ok && name == key {
			return value
		}
	}
	return ""
}

// randomUUID builds a version 4 UUID string.
func randomUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand never fails on Linux; fall back to a time-derived value.
		binaryPutTime(b[:], time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32])
}

func binaryPutTime(dst []byte, v int64) {
	for i := range dst {
		dst[i] = byte(v >> (uint(i%8) * 8))
	}
}

// randomAlnum returns n random alphanumeric characters, used for the fake
// OAuth secrets the AuthenticationService hands out.
func randomAlnum(n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		binaryPutTime(buf, time.Now().UnixNano())
	}
	for i := range buf {
		buf[i] = alphabet[int(buf[i])%len(alphabet)]
	}
	return string(buf)
}
