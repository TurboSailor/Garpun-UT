package garminhttp

import (
	"fmt"
	"net/url"
	"strings"

	pb "pulse/backend/internal/gproto/garmin"
)

// Request is a watch-originated HTTP request, normalised the same way the
// reference implementation does: header keys lowercased, url parsed, body
// extracted from whichever request flavour arrived.
type Request struct {
	RequestID uint16
	Raw       *pb.HttpService_RawRequest
	Web       *pb.HttpService_WebRequest

	Method  string
	RawURL  string
	URL     *url.URL
	Headers map[string]string
	Query   url.Values
	Body    []byte
}

func newRawRequest(requestID uint16, raw *pb.HttpService_RawRequest) (*Request, error) {
	u, err := url.Parse(raw.GetUrl())
	if err != nil {
		return nil, fmt.Errorf("garminhttp: parse raw url %q: %w", raw.GetUrl(), err)
	}
	headers := make(map[string]string, len(raw.GetHeader()))
	for _, h := range raw.GetHeader() {
		headers[strings.ToLower(h.GetKey())] = h.GetValue()
	}
	return &Request{
		RequestID: requestID,
		Raw:       raw,
		Method:    methodName(raw.GetMethod()),
		RawURL:    raw.GetUrl(),
		URL:       u,
		Headers:   headers,
		Query:     u.Query(),
		Body:      raw.GetRawBody(),
	}, nil
}

func newWebRequest(requestID uint16, web *pb.HttpService_WebRequest) (*Request, error) {
	u, err := url.Parse(web.GetUrl())
	if err != nil {
		return nil, fmt.Errorf("garminhttp: parse web url %q: %w", web.GetUrl(), err)
	}
	headers := make(map[string]string)
	if len(web.GetHeaders()) > 0 {
		decoded, err := DecodeGarminJSON(web.GetHeaders())
		if err != nil {
			return nil, fmt.Errorf("garminhttp: decode web headers: %w", err)
		}
		if obj, ok := decoded.(*JSONObject); ok {
			for _, k := range obj.Keys {
				if v, ok := obj.GetString(k); ok {
					headers[strings.ToLower(k)] = v
				}
			}
		}
	}
	return &Request{
		RequestID: requestID,
		Web:       web,
		Method:    methodName(web.GetMethod()),
		RawURL:    web.GetUrl(),
		URL:       u,
		Headers:   headers,
		Query:     u.Query(),
		Body:      web.GetBody(),
	}, nil
}

// Host is the request domain without a port.
func (r *Request) Host() string {
	return r.URL.Hostname()
}

// Path is the url path.
func (r *Request) Path() string {
	return r.URL.Path
}

// Header returns a request header by its lowercase name.
func (r *Request) Header(name string) string {
	return r.Headers[name]
}

// QueryInt returns an integer query parameter or def when absent/unparsable.
func (r *Request) QueryInt(name string, def int) int {
	v := r.Query.Get(name)
	if v == "" {
		return def
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return def
	}
	return n
}

// QueryFloat returns a float query parameter or def when absent/unparsable.
func (r *Request) QueryFloat(name string, def float64) float64 {
	v := r.Query.Get(name)
	if v == "" {
		return def
	}
	var f float64
	if _, err := fmt.Sscanf(v, "%g", &f); err != nil {
		return def
	}
	return f
}

func methodName(m pb.HttpService_Method) string {
	if name, ok := pb.HttpService_Method_name[int32(m)]; ok {
		return name
	}
	return pb.HttpService_Method_name[int32(pb.HttpService_UNKNOWN_METHOD)]
}

// Response is what an interceptor produces. Header order is preserved because
// the legacy webResponse encodes them as an ordered Garmin JSON object.
type Response struct {
	Status  int
	Body    []byte
	headers []headerPair
	// complete is false when the reply will be pushed asynchronously.
	complete bool
	// onSent fires once the watch has pulled the whole data transfer payload.
	onSent func()
}

type headerPair struct {
	key   string
	value string
}

func newResponse(status int) *Response {
	return &Response{Status: status, complete: true}
}

// SetHeader adds or replaces a response header.
func (r *Response) SetHeader(key, value string) {
	for i := range r.headers {
		if strings.EqualFold(r.headers[i].key, key) {
			r.headers[i].value = value
			return
		}
	}
	r.headers = append(r.headers, headerPair{key, value})
}

// Header looks a response header up case-insensitively.
func (r *Response) Header(key string) string {
	for i := range r.headers {
		if strings.EqualFold(r.headers[i].key, key) {
			return r.headers[i].value
		}
	}
	return ""
}
