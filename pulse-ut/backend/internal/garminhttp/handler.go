package garminhttp

import (
	"bytes"
	"compress/gzip"
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	pb "pulse/backend/internal/gproto/garmin"
)

// WeatherSource supplies the JSON payloads for the weather endpoints the watch
// polls through the HTTP proxy. The returned values are marshalled as-is.
type WeatherSource interface {
	Current(ctx context.Context, lat, lon float64) (any, error)
	Hourly(ctx context.Context, lat, lon float64, hours int) (any, error)
	Daily(ctx context.Context, lat, lon float64, days int) (any, error)
}

// PointWindsSource is an optional extension of WeatherSource. When the weather
// implementation provides it, /weather/pointWinds is served too.
type PointWindsSource interface {
	PointWinds(ctx context.Context, lat, lon float64) (any, error)
}

// LocationSource is the phone's last known position, in degrees.
type LocationSource interface {
	Location() (lat, lon float64, ok bool)
}

// Options configures the protobuf service handler.
type Options struct {
	// Weather answers /weather/v1 and /weather/v2 requests; nil replies 404.
	Weather WeatherSource
	// Location answers CoreService location requests; nil replies with
	// NO_VALID_LOCATION.
	Location LocationSource
	// OnBattery receives watch battery percentages from DeviceStatusService.
	OnBattery func(level int32)
	// CannedReplies are offered to the watch for SMS and call rejections.
	CannedReplies []string
	// AgpsURL maps the url the watch asked for onto the url to actually fetch
	// the ephemeris blob from. Returning "" refuses the request. Nil keeps the
	// requested url.
	AgpsURL func(requestURL string) string
	// Client is used for AGPS and generic pass-through requests.
	Client *http.Client
	// Timeout bounds a single outbound request (default 30s).
	Timeout time.Duration
	// MaxResponseBytes caps a pass-through response body (default 1 MiB).
	MaxResponseBytes int
	// OnSettings receives raw realtime-settings protobuf payloads coming back
	// from the watch, keyed by "definition", "state" or "change".
	OnSettings func(kind string, payload []byte)
}

// Handler implements garmin.Hooks.Protobuf.
type Handler struct {
	log  *slog.Logger
	opts Options

	client    *http.Client
	transfers *dataTransfer
	chain     []interceptor

	mu   sync.Mutex
	send func(requestID uint16, payload []byte)
	agps map[string]*agpsEntry
}

// New builds a handler. The logger must not be nil.
func New(log *slog.Logger, opts Options) *Handler {
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.MaxResponseBytes <= 0 {
		opts.MaxResponseBytes = 1 << 20
	}
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: opts.Timeout}
	}
	h := &Handler{
		log:       log,
		opts:      opts,
		client:    client,
		transfers: newDataTransfer(),
		agps:      make(map[string]*agpsEntry),
	}
	// Mirrors the reference chain: the first interceptor that claims the
	// request answers it, and the firewall is always last.
	h.chain = []interceptor{
		{"weather", isWeatherRequest, h.handleWeather},
		{"agps", isAgpsRequest, h.handleAgps},
		{"oauth", isOauthRequest, h.handleOauth},
		{"contacts", isContactsRequest, h.handleContacts},
		{"firewall", func(*Request) bool { return true }, h.handleFirewall},
	}
	return h
}

// SetSender installs the callback used for replies that cannot be produced
// synchronously. The watch matches an HTTP reply by the request id of the
// original PROTOBUF_REQUEST, so it has to be carried through.
func (h *Handler) SetSender(send func(requestID uint16, payload []byte)) {
	h.mu.Lock()
	h.send = send
	h.mu.Unlock()
}

func (h *Handler) sender() func(requestID uint16, payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.send
}

// Handle decodes one Smart message and optionally returns a serialised reply.
func (h *Handler) Handle(requestID uint16, payload []byte) ([]byte, error) {
	var smart pb.Smart
	if err := proto.Unmarshal(payload, &smart); err != nil {
		return nil, err
	}

	reply := h.dispatch(requestID, &smart)
	if reply == nil {
		return nil, nil
	}
	return proto.Marshal(reply)
}

func (h *Handler) dispatch(requestID uint16, smart *pb.Smart) *pb.Smart {
	switch {
	case smart.GetCoreService() != nil:
		return h.handleCore(smart.GetCoreService())
	case smart.GetCalendarService() != nil:
		return h.handleCalendar(smart.GetCalendarService())
	case smart.GetSmsNotificationService() != nil:
		return h.handleSms(smart.GetSmsNotificationService())
	case smart.GetHttpService() != nil:
		return h.handleHTTP(requestID, smart.GetHttpService())
	case smart.GetDataTransferService() != nil:
		return h.handleDataTransfer(smart.GetDataTransferService())
	case smart.GetDeviceStatusService() != nil:
		h.handleDeviceStatus(smart.GetDeviceStatusService())
		return nil
	case smart.GetFindMyWatchService() != nil:
		h.handleFindMyWatch(smart.GetFindMyWatchService())
		return nil
	case smart.GetSettingsService() != nil:
		h.handleSettings(smart.GetSettingsService())
		return nil
	case smart.GetAuthenticationService() != nil:
		return h.handleAuthentication(smart.GetAuthenticationService())
	case smart.GetNotificationsService() != nil:
		return h.handleNotifications(smart.GetNotificationsService())
	default:
		h.log.Warn("garminhttp: unhandled protobuf service", "service", serviceName(smart))
		return nil
	}
}

// serviceName names the populated field for logging, without dumping payloads.
func serviceName(smart *pb.Smart) string {
	switch {
	case smart.GetInstalledAppsService() != nil:
		return "installedApps"
	case smart.GetAppConfigService() != nil:
		return "appConfig"
	case smart.GetFileSyncService() != nil:
		return "fileSync"
	case smart.GetEcgService() != nil:
		return "ecg"
	case smart.GetExploreSyncService() != nil:
		return "exploreSync"
	default:
		return "unknown"
	}
}

// interceptor is one link of the HTTP chain built in New.
type interceptor struct {
	name    string
	matches func(*Request) bool
	handle  func(*Request) *Response
}

func (h *Handler) handleHTTP(requestID uint16, svc *pb.HttpService) *pb.Smart {
	switch {
	case svc.GetRawRequest() != nil:
		req, err := newRawRequest(requestID, svc.GetRawRequest())
		if err != nil {
			h.log.Warn("garminhttp: bad raw request", "err", err)
			return errorSmart(&Request{Raw: svc.GetRawRequest()})
		}
		h.log.Debug("garminhttp: rawRequest", "id", requestID, "method", req.Method, "url", req.RawURL)
		return h.runInterceptors(req)

	case svc.GetWebRequest() != nil:
		req, err := newWebRequest(requestID, svc.GetWebRequest())
		if err != nil {
			h.log.Warn("garminhttp: bad web request", "err", err)
			return errorSmart(&Request{Web: svc.GetWebRequest()})
		}
		h.log.Debug("garminhttp: webRequest", "id", requestID, "method", req.Method, "url", req.RawURL)
		return h.runInterceptors(req)

	case svc.GetShowURLRequest() != nil:
		h.log.Warn("garminhttp: unsupported showURLRequest",
			"url", svc.GetShowURLRequest().GetUrl(),
			"app", svc.GetShowURLRequest().GetApp())
		return nil

	default:
		h.log.Warn("garminhttp: empty http service request")
		return nil
	}
}

func (h *Handler) runInterceptors(req *Request) *pb.Smart {
	for _, in := range h.chain {
		if !in.matches(req) {
			continue
		}
		resp := in.handle(req)
		if resp == nil {
			h.log.Debug("garminhttp: interceptor refused", "interceptor", in.name, "path", req.Path())
			return errorSmart(req)
		}
		if !resp.complete {
			// The reply will arrive through the sender.
			return nil
		}
		h.log.Debug("garminhttp: interceptor answered",
			"interceptor", in.name, "status", resp.Status, "bytes", len(resp.Body))
		return h.successSmart(req, resp)
	}
	return errorSmart(req)
}

// errorSmart is the "we did not answer" reply shape for both request flavours.
func errorSmart(req *Request) *pb.Smart {
	status := pb.HttpService_UNKNOWN_STATUS
	if req.Web != nil {
		httpStatus := uint32(0)
		return &pb.Smart{HttpService: &pb.HttpService{
			WebResponse: &pb.HttpService_WebResponse{Status: &status, HttpStatus: &httpStatus},
		}}
	}
	return &pb.Smart{HttpService: &pb.HttpService{
		RawResponse: &pb.HttpService_RawResponse{Status: &status},
	}}
}

func (h *Handler) successSmart(req *Request, resp *Response) *pb.Smart {
	if req.Web != nil {
		web := h.buildWebResponse(req, resp)
		if web == nil {
			return errorSmart(req)
		}
		return &pb.Smart{HttpService: &pb.HttpService{WebResponse: web}}
	}
	raw := h.buildRawResponse(req, resp)
	if raw == nil {
		return errorSmart(req)
	}
	return &pb.Smart{HttpService: &pb.HttpService{RawResponse: raw}}
}

func (h *Handler) buildRawResponse(req *Request, resp *Response) *pb.HttpService_RawResponse {
	headers := make([]*pb.HttpService_Header, 0, len(resp.headers)+1)
	for i := range resp.headers {
		headers = append(headers, header(resp.headers[i].key, resp.headers[i].value))
	}

	status := pb.HttpService_OK
	httpStatus := uint32(resp.Status)

	if req.Raw.GetUseDataXfer() {
		// The body travels through DataTransferService instead of the reply.
		id := h.transfers.register(resp.Body, resp.onSent)
		size := uint32(len(resp.Body))
		return &pb.HttpService_RawResponse{
			Status:     &status,
			HttpStatus: &httpStatus,
			Header:     headers,
			XferData:   &pb.HttpService_DataTransferItem{Id: &id, Size: &size},
		}
	}

	body := resp.Body
	// The watch compares the full header value, so only a bare "gzip" counts.
	if req.Header("accept-encoding") == "gzip" {
		compressed, err := gzipBytes(body)
		if err != nil {
			h.log.Error("garminhttp: compress response", "err", err)
			return nil
		}
		body = compressed
		headers = append(headers, header("Content-Encoding", "gzip"))
	}
	if resp.onSent != nil {
		// Nothing to wait for without a data transfer.
		resp.onSent()
	}
	return &pb.HttpService_RawResponse{
		Status:     &status,
		HttpStatus: &httpStatus,
		Body:       body,
		Header:     headers,
	}
}

func (h *Handler) buildWebResponse(req *Request, resp *Response) *pb.HttpService_WebResponse {
	zero := uint32(0)
	httpStatus := uint32(resp.Status)

	var headerBytes []byte
	if req.Web.GetHttpHeadersInResponse() {
		obj := &JSONObject{}
		for i := range resp.headers {
			obj.Set(resp.headers[i].key, resp.headers[i].value)
		}
		encoded, err := EncodeGarminJSON(obj)
		if err != nil {
			h.log.Error("garminhttp: encode web headers", "err", err)
		} else {
			headerBytes = encoded
		}
	}

	if uint32(len(resp.Body)) > req.Web.GetMaxResponseLength() {
		// Compression of oversized bodies is not part of the protocol we know.
		status := pb.HttpService_FILE_TOO_LARGE
		return &pb.HttpService_WebResponse{Status: &status, HttpStatus: &zero}
	}

	var value any
	contentType := resp.Header("content-type")
	switch {
	case strings.HasPrefix(contentType, "application/json"):
		parsed, err := ParseJSON(resp.Body)
		if err != nil {
			h.log.Error("garminhttp: decode body as json", "err", err)
			status := pb.HttpService_DATA_TRANSFER_ITEM_FAILURE
			return &pb.HttpService_WebResponse{Status: &status, HttpStatus: &zero}
		}
		value = parsed
	case req.Web.GetResponseType() == pb.HttpService_JSON:
		h.log.Warn("garminhttp: json expected but body is not json", "contentType", contentType)
		status := pb.HttpService_DATA_TRANSFER_ITEM_FAILURE
		return &pb.HttpService_WebResponse{Status: &status, HttpStatus: &httpStatus}
	default:
		value = string(resp.Body)
	}

	body, err := EncodeGarminJSON(value)
	if err != nil {
		h.log.Error("garminhttp: encode web body", "err", err)
		return nil
	}
	if resp.onSent != nil {
		resp.onSent()
	}

	status := pb.HttpService_OK
	return &pb.HttpService_WebResponse{
		Status:     &status,
		HttpStatus: &httpStatus,
		Body:       body,
		Headers:    headerBytes,
		// 0 marks an uncompressed body.
		Size: &zero,
	}
}

// sendAsync pushes a late reply for req through the sender.
func (h *Handler) sendAsync(req *Request, resp *Response) {
	send := h.sender()
	if send == nil {
		h.log.Warn("garminhttp: no sender for async reply", "id", req.RequestID)
		return
	}
	var smart *pb.Smart
	if resp == nil {
		smart = errorSmart(req)
	} else {
		smart = h.successSmart(req, resp)
	}
	payload, err := proto.Marshal(smart)
	if err != nil {
		h.log.Error("garminhttp: marshal async reply", "err", err)
		return
	}
	send(req.RequestID, payload)
}

func header(key, value string) *pb.HttpService_Header {
	return &pb.HttpService_Header{Key: &key, Value: &value}
}

func gzipBytes(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
