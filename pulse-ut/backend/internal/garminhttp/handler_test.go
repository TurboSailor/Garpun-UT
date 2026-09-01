package garminhttp

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"pulse/backend/internal/garmin"
	pb "pulse/backend/internal/gproto/garmin"
)

// Handle must be usable as garmin.Hooks.Protobuf without an adapter.
func TestHandleFitsProtobufHook(t *testing.T) {
	h := testHandler(t, Options{})
	hooks := garmin.Hooks{Protobuf: h.Handle}
	if hooks.Protobuf == nil {
		t.Fatal("hook not installed")
	}
}

type fakeWeather struct {
	lastLat, lastLon float64
	lastHours        int
}

func (f *fakeWeather) Current(_ context.Context, lat, lon float64) (any, error) {
	f.lastLat, f.lastLon = lat, lon
	return map[string]any{"temperature": 21}, nil
}

func (f *fakeWeather) Hourly(_ context.Context, lat, lon float64, hours int) (any, error) {
	f.lastLat, f.lastLon, f.lastHours = lat, lon, hours
	return []any{map[string]any{"epochSeconds": 1}}, nil
}

func (f *fakeWeather) Daily(_ context.Context, _, _ float64, days int) (any, error) {
	f.lastHours = days
	return []any{map[string]any{"dayOfWeek": days}}, nil
}

type fixedLocation struct {
	lat, lon float64
	ok       bool
}

func (f fixedLocation) Location() (float64, float64, bool) { return f.lat, f.lon, f.ok }

func handleSmart(t *testing.T, h *Handler, requestID uint16, msg *pb.Smart) *pb.Smart {
	t.Helper()
	in, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	out, err := h.Handle(requestID, in)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if out == nil {
		return nil
	}
	var reply pb.Smart
	if err := proto.Unmarshal(out, &reply); err != nil {
		t.Fatalf("unmarshal reply: %v", err)
	}
	return &reply
}

func rawSmart(url string, headers map[string]string, useDataXfer bool) *pb.Smart {
	method := pb.HttpService_GET
	raw := &pb.HttpService_RawRequest{Url: &url, Method: &method}
	for k, v := range headers {
		key, value := k, v
		raw.Header = append(raw.Header, &pb.HttpService_Header{Key: &key, Value: &value})
	}
	if useDataXfer {
		raw.UseDataXfer = &useDataXfer
	}
	return &pb.Smart{HttpService: &pb.HttpService{RawRequest: raw}}
}

func TestHandleWeatherRawRequest(t *testing.T) {
	weather := &fakeWeather{}
	h := testHandler(t, Options{Weather: weather})

	reply := handleSmart(t, h, 42,
		rawSmart("https://api.gcs.garmin.com/weather/v2/forecast/hour?lat=55.75&lon=37.62&duration=9", nil, false))

	resp := reply.GetHttpService().GetRawResponse()
	if resp == nil {
		t.Fatal("expected a rawResponse")
	}
	if resp.GetStatus() != pb.HttpService_OK || resp.GetHttpStatus() != 200 {
		t.Fatalf("status = %v / %d", resp.GetStatus(), resp.GetHttpStatus())
	}
	var body []any
	if err := json.Unmarshal(resp.GetBody(), &body); err != nil {
		t.Fatalf("body is not the weather json: %v (%s)", err, resp.GetBody())
	}
	if weather.lastHours != 9 {
		t.Fatalf("duration = %d, want 9", weather.lastHours)
	}
	if weather.lastLat != 55.75 || weather.lastLon != 37.62 {
		t.Fatalf("coordinates = %v, %v", weather.lastLat, weather.lastLon)
	}
	if headerValue(resp.GetHeader(), "Content-Type") != "application/json" {
		t.Fatalf("headers = %v", resp.GetHeader())
	}
}

func TestHandleRawRequestGzipsWhenAsked(t *testing.T) {
	h := testHandler(t, Options{Weather: &fakeWeather{}})

	reply := handleSmart(t, h, 1, rawSmart(
		"https://api.gcs.garmin.com/weather/v1/current?lat=1&lon=2",
		map[string]string{"Accept-Encoding": "gzip"},
		false,
	))
	resp := reply.GetHttpService().GetRawResponse()
	if headerValue(resp.GetHeader(), "Content-Encoding") != "gzip" {
		t.Fatalf("missing Content-Encoding: %v", resp.GetHeader())
	}
	zr, err := gzip.NewReader(bytes.NewReader(resp.GetBody()))
	if err != nil {
		t.Fatalf("body is not gzip: %v", err)
	}
	plain, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gunzip: %v", err)
	}
	if !bytes.Contains(plain, []byte(`"temperature":21`)) {
		t.Fatalf("decompressed body = %s", plain)
	}
}

func TestHandleRawRequestUsesDataTransfer(t *testing.T) {
	h := testHandler(t, Options{Weather: &fakeWeather{}})

	reply := handleSmart(t, h, 5, rawSmart(
		"https://api.gcs.garmin.com/weather/v1/current?lat=1&lon=2", nil, true))
	resp := reply.GetHttpService().GetRawResponse()
	if len(resp.GetBody()) != 0 {
		t.Fatalf("body must travel through data transfer, got %d bytes", len(resp.GetBody()))
	}
	xfer := resp.GetXferData()
	if xfer == nil || xfer.GetSize() == 0 {
		t.Fatalf("missing xferData: %v", resp)
	}

	pull := handleSmart(t, h, 6, &pb.Smart{DataTransferService: downloadRequest(xfer.GetId(), 0, 375)})
	got := pull.GetDataTransferService().GetDataDownloadResponse()
	if got.GetStatus() != pb.DataTransferService_SUCCESS {
		t.Fatalf("download status = %v", got.GetStatus())
	}
	if uint32(len(got.GetPayload())) != xfer.GetSize() {
		t.Fatalf("payload is %d bytes, announced %d", len(got.GetPayload()), xfer.GetSize())
	}
}

func TestHandleWebRequestEncodesGarminJSON(t *testing.T) {
	h := testHandler(t, Options{Weather: &fakeWeather{}})

	url := "https://api.gcs.garmin.com/weather/v1/current?lat=1&lon=2"
	method := pb.HttpService_GET
	maxLen := uint32(4096)
	inResponse := true
	web := &pb.HttpService_WebRequest{
		Url:                   &url,
		Method:                &method,
		MaxResponseLength:     &maxLen,
		HttpHeadersInResponse: &inResponse,
	}
	reply := handleSmart(t, h, 9, &pb.Smart{HttpService: &pb.HttpService{WebRequest: web}})

	resp := reply.GetHttpService().GetWebResponse()
	if resp == nil || resp.GetStatus() != pb.HttpService_OK {
		t.Fatalf("unexpected web response %v", resp)
	}
	if resp.GetSize() != 0 {
		t.Fatalf("size = %d, want 0 for uncompressed", resp.GetSize())
	}
	decoded, err := DecodeGarminJSON(resp.GetBody())
	if err != nil {
		t.Fatalf("decode body: %v", err)
	}
	obj, ok := decoded.(*JSONObject)
	if !ok {
		t.Fatalf("body decoded as %T, want an object", decoded)
	}
	if v, _ := obj.Get("temperature"); v != json.Number("21") {
		t.Fatalf("temperature = %#v", v)
	}
	headers, err := DecodeGarminJSON(resp.GetHeaders())
	if err != nil {
		t.Fatalf("decode headers: %v", err)
	}
	headerObj, ok := headers.(*JSONObject)
	if !ok {
		t.Fatalf("headers decoded as %T", headers)
	}
	if v, _ := headerObj.GetString("Content-Type"); v != "application/json" {
		t.Fatalf("header content-type = %q", v)
	}
}

func TestHandleWebRequestTooLarge(t *testing.T) {
	h := testHandler(t, Options{Weather: &fakeWeather{}})

	url := "https://api.gcs.garmin.com/weather/v1/current?lat=1&lon=2"
	method := pb.HttpService_GET
	maxLen := uint32(4)
	web := &pb.HttpService_WebRequest{Url: &url, Method: &method, MaxResponseLength: &maxLen}
	reply := handleSmart(t, h, 10, &pb.Smart{HttpService: &pb.HttpService{WebRequest: web}})

	resp := reply.GetHttpService().GetWebResponse()
	if resp.GetStatus() != pb.HttpService_FILE_TOO_LARGE {
		t.Fatalf("status = %v, want FILE_TOO_LARGE", resp.GetStatus())
	}
}

func TestHandleBlockedRequestReturnsUnknownStatus(t *testing.T) {
	h := testHandler(t, Options{})
	reply := handleSmart(t, h, 3, rawSmart("https://connectapi.garmin.com/anything", nil, false))
	if got := reply.GetHttpService().GetRawResponse().GetStatus(); got != pb.HttpService_UNKNOWN_STATUS {
		t.Fatalf("status = %v, want UNKNOWN_STATUS", got)
	}
}

func TestHandleProxiesForeignHostAsynchronously(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Watch") != "yes" {
			t.Errorf("request headers not forwarded: %v", r.Header)
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("hello watch"))
	}))
	defer srv.Close()

	h := testHandler(t, Options{})
	replies := make(chan []byte, 1)
	h.SetSender(func(requestID uint16, payload []byte) {
		if requestID != 77 {
			t.Errorf("async reply carries request id %d, want 77", requestID)
		}
		replies <- payload
	})

	reply := handleSmart(t, h, 77, rawSmart(srv.URL, map[string]string{"X-Watch": "yes"}, false))
	if reply != nil {
		t.Fatalf("proxied request must answer asynchronously, got %v", reply)
	}

	select {
	case payload := <-replies:
		var smart pb.Smart
		if err := proto.Unmarshal(payload, &smart); err != nil {
			t.Fatalf("unmarshal async reply: %v", err)
		}
		resp := smart.GetHttpService().GetRawResponse()
		if resp.GetStatus() != pb.HttpService_OK || resp.GetHttpStatus() != 200 {
			t.Fatalf("async status = %v / %d", resp.GetStatus(), resp.GetHttpStatus())
		}
		if string(resp.GetBody()) != "hello watch" {
			t.Fatalf("async body = %q", resp.GetBody())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the async reply")
	}
}

func TestHandleProxyFailureReportsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing is listening any more

	h := testHandler(t, Options{Timeout: time.Second})
	replies := make(chan []byte, 1)
	h.SetSender(func(_ uint16, payload []byte) { replies <- payload })

	handleSmart(t, h, 8, rawSmart(url, nil, false))
	select {
	case payload := <-replies:
		var smart pb.Smart
		if err := proto.Unmarshal(payload, &smart); err != nil {
			t.Fatalf("unmarshal async reply: %v", err)
		}
		if got := smart.GetHttpService().GetRawResponse().GetStatus(); got != pb.HttpService_UNKNOWN_STATUS {
			t.Fatalf("status = %v, want UNKNOWN_STATUS", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the error reply")
	}
}

func TestHandleCoreLocation(t *testing.T) {
	h := testHandler(t, Options{Location: fixedLocation{lat: 55.75, lon: 37.62, ok: true}})
	kind := pb.CoreService_GetLocationRequest_STANDARD
	reply := handleSmart(t, h, 11, &pb.Smart{CoreService: &pb.CoreService{
		GetLocationRequest: &pb.CoreService_GetLocationRequest{RequestType: &kind},
	}})

	resp := reply.GetCoreService().GetGetLocationResponse()
	if resp.GetStatus() != pb.CoreService_GetLocationResponse_OK {
		t.Fatalf("status = %v", resp.GetStatus())
	}
	data := resp.GetLocationData()
	if data.GetPositionType() != pb.CoreService_GENERAL_LOCATION {
		t.Fatalf("position type = %v", data.GetPositionType())
	}
	// 55.75 degrees in semicircles.
	if want := int32(665123408); data.GetPosition().GetLat() != want {
		t.Fatalf("lat = %d, want %d", data.GetPosition().GetLat(), want)
	}
	if got := float64(data.GetPosition().GetLon()) / semicircleScale; got < 37.61 || got > 37.63 {
		t.Fatalf("lon round trip = %v", got)
	}
}

func TestHandleCoreLocationWithoutSource(t *testing.T) {
	h := testHandler(t, Options{})
	reply := handleSmart(t, h, 12, &pb.Smart{CoreService: &pb.CoreService{
		GetLocationRequest: &pb.CoreService_GetLocationRequest{},
	}})
	resp := reply.GetCoreService().GetGetLocationResponse()
	if resp.GetStatus() != pb.CoreService_GetLocationResponse_NO_VALID_LOCATION {
		t.Fatalf("status = %v, want NO_VALID_LOCATION", resp.GetStatus())
	}
	if resp.GetLocationData() != nil {
		t.Fatal("no location data expected")
	}
}

func TestHandleCoreLocationUpdatesDeclined(t *testing.T) {
	h := testHandler(t, Options{Location: fixedLocation{lat: 1, lon: 2, ok: true}})
	enabled := true
	realtime := pb.CoreService_REALTIME_TRACKING
	general := pb.CoreService_GENERAL_LOCATION
	reply := handleSmart(t, h, 13, &pb.Smart{CoreService: &pb.CoreService{
		LocationUpdatedSetEnabledRequest: &pb.CoreService_LocationUpdatedSetEnabledRequest{
			Enabled: &enabled,
			Requests: []*pb.CoreService_Request{
				{Requested: &realtime},
				{Requested: &general},
			},
		},
	}})

	resp := reply.GetCoreService().GetLocationUpdatedSetEnabledResponse()
	if resp.GetStatus() != pb.CoreService_LocationUpdatedSetEnabledResponse_OK {
		t.Fatalf("envelope status = %v", resp.GetStatus())
	}
	if len(resp.GetRequests()) != 2 {
		t.Fatalf("got %d per-request statuses", len(resp.GetRequests()))
	}
	for _, r := range resp.GetRequests() {
		if r.GetStatus() != pb.CoreService_LocationUpdatedSetEnabledResponse_Requested_KO {
			t.Fatalf("%v accepted, want KO", r.GetRequested())
		}
	}
}

func TestHandleDeviceStatusBattery(t *testing.T) {
	got := int32(-1)
	h := testHandler(t, Options{OnBattery: func(level int32) { got = level }})

	status := pb.DeviceStatusService_OK
	level := int32(63)
	reply := handleSmart(t, h, 14, &pb.Smart{DeviceStatusService: &pb.DeviceStatusService{
		RemoteDeviceBatteryStatusResponse: &pb.DeviceStatusService_RemoteDeviceBatteryStatusResponse{
			Status:              &status,
			CurrentBatteryLevel: &level,
		},
	}})
	if reply != nil {
		t.Fatalf("battery status needs no reply, got %v", reply)
	}
	if got != 63 {
		t.Fatalf("OnBattery got %d, want 63", got)
	}
}

func TestHandleCannedList(t *testing.T) {
	h := testHandler(t, Options{CannedReplies: []string{"Ok", "Later"}})
	requested := []pb.SmsNotificationService_CannedListType{
		pb.SmsNotificationService_PHONE_CALL_RESPONSE,
		pb.SmsNotificationService_SMS_MESSAGE_RESPONSE,
	}
	reply := handleSmart(t, h, 15, &pb.Smart{SmsNotificationService: &pb.SmsNotificationService{
		SmsCannedListRequest: &pb.SmsNotificationService_SmsCannedListRequest{RequestedTypes: requested},
	}})

	resp := reply.GetSmsNotificationService().GetSmsCannedListResponse()
	if resp.GetStatus() != pb.SmsNotificationService_SUCCESS {
		t.Fatalf("status = %v", resp.GetStatus())
	}
	if len(resp.GetLists()) != 2 {
		t.Fatalf("got %d lists", len(resp.GetLists()))
	}
	for i, list := range resp.GetLists() {
		if list.GetType() != requested[i] {
			t.Fatalf("list %d type = %v", i, list.GetType())
		}
		if len(list.GetResponse()) != 2 || list.GetResponse()[1] != "Later" {
			t.Fatalf("list %d responses = %v", i, list.GetResponse())
		}
	}
}

func TestHandleCannedListWithoutReplies(t *testing.T) {
	h := testHandler(t, Options{})
	reply := handleSmart(t, h, 16, &pb.Smart{SmsNotificationService: &pb.SmsNotificationService{
		SmsCannedListRequest: &pb.SmsNotificationService_SmsCannedListRequest{
			RequestedTypes: []pb.SmsNotificationService_CannedListType{
				pb.SmsNotificationService_SMS_MESSAGE_RESPONSE,
			},
		},
	}})
	if got := reply.GetSmsNotificationService().GetSmsCannedListResponse().GetStatus(); got != pb.SmsNotificationService_GENERIC_ERROR {
		t.Fatalf("status = %v, want GENERIC_ERROR", got)
	}
}

func TestHandleAuthenticationFakeKeys(t *testing.T) {
	h := testHandler(t, Options{})
	reply := handleSmart(t, h, 17, &pb.Smart{AuthenticationService: &pb.AuthenticationService{
		OauthRequest: &pb.OAuthRequest{},
	}})
	keys := reply.GetAuthenticationService().GetOauthResponse().GetKeys()
	if len(keys.GetConsumerKey()) != 36 || len(keys.GetOauthToken()) != 36 {
		t.Fatalf("uuid keys look wrong: %q / %q", keys.GetConsumerKey(), keys.GetOauthToken())
	}
	if len(keys.GetConsumerSecret()) != 35 || len(keys.GetOauthSecret()) != 35 {
		t.Fatalf("secrets look wrong: %q / %q", keys.GetConsumerSecret(), keys.GetOauthSecret())
	}
}

func TestHandleCalendarEmptyList(t *testing.T) {
	h := testHandler(t, Options{})
	begin, end := uint32(1), uint32(2)
	reply := handleSmart(t, h, 18, &pb.Smart{CalendarService: &pb.CalendarService{
		CalendarRequest: &pb.CalendarService_CalendarServiceRequest{Begin: &begin, End: &end},
	}})
	resp := reply.GetCalendarService().GetCalendarResponse()
	if resp.GetStatus() != pb.CalendarService_CalendarServiceResponse_OK {
		t.Fatalf("status = %v", resp.GetStatus())
	}
	if len(resp.GetCalendarEvent()) != 0 {
		t.Fatalf("expected no events, got %d", len(resp.GetCalendarEvent()))
	}
}

func TestHandleNotificationPictureDeclined(t *testing.T) {
	h := testHandler(t, Options{})
	id := uint32(4)
	reply := handleSmart(t, h, 19, &pb.Smart{NotificationsService: &pb.NotificationsService{
		PictureRequest: &pb.PictureRequest{NotificationId: &id},
	}})
	if reply != nil {
		t.Fatalf("picture request must be declined silently, got %v", reply)
	}
}

func TestHandleSettingsForwarded(t *testing.T) {
	var kinds []string
	h := testHandler(t, Options{OnSettings: func(kind string, payload []byte) {
		if len(payload) == 0 {
			t.Errorf("%s payload is empty", kind)
		}
		kinds = append(kinds, kind)
	}})

	screenID := uint32(1)
	reply := handleSmart(t, h, 20, &pb.Smart{SettingsService: &pb.SettingsService{
		StateResponse: &pb.ScreenStateResponse{
			State: &pb.ScreenState{ScreenId: &screenID},
		},
	}})
	if reply != nil {
		t.Fatalf("settings responses need no reply, got %v", reply)
	}
	if len(kinds) != 1 || kinds[0] != "state" {
		t.Fatalf("forwarded kinds = %v", kinds)
	}
}

func TestHandleUnknownServiceIsIgnored(t *testing.T) {
	h := testHandler(t, Options{})
	reply := handleSmart(t, h, 21, &pb.Smart{FileSyncService: &pb.FileSyncService{}})
	if reply != nil {
		t.Fatalf("unhandled service must not produce a reply, got %v", reply)
	}
}

func TestHandleRejectsBrokenPayload(t *testing.T) {
	h := testHandler(t, Options{})
	if _, err := h.Handle(1, []byte{0xff, 0xff, 0xff}); err == nil {
		t.Fatal("expected a parse error")
	}
}

func headerValue(headers []*pb.HttpService_Header, key string) string {
	for _, h := range headers {
		if h.GetKey() == key {
			return h.GetValue()
		}
	}
	return ""
}
