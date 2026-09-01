package garminhttp

import (
	"context"
	"encoding/json"
	"strings"
)

// The watch polls these on connection, at most every five minutes.

func isWeatherRequest(req *Request) bool {
	host := req.Host()
	if host != "api.gcs.garmin.com" && host != "cache.dciwx.com" {
		return false
	}
	return strings.HasPrefix(req.Path(), "/weather/")
}

func (h *Handler) handleWeather(req *Request) *Response {
	if h.opts.Weather == nil {
		h.log.Warn("garminhttp: weather requested but no source configured", "path", req.Path())
		return notFound()
	}

	path := req.Path()
	version := 1
	var endpoint string
	switch {
	case strings.HasPrefix(path, "/weather/v2/"):
		version, endpoint = 2, strings.TrimPrefix(path, "/weather/v2")
	case strings.HasPrefix(path, "/weather/v1/"):
		version, endpoint = 1, strings.TrimPrefix(path, "/weather/v1")
	case path == "/weather/pointWinds":
		endpoint = "/pointWinds"
	default:
		h.log.Warn("garminhttp: unsupported weather path", "path", path)
		return nil
	}

	lat, lon := weatherCoords(req.QueryFloat("lat", 0), req.QueryFloat("lon", 0))

	ctx, cancel := context.WithTimeout(context.Background(), h.opts.Timeout)
	defer cancel()

	var (
		data any
		err  error
	)
	switch endpoint {
	case "/current":
		data, err = h.opts.Weather.Current(ctx, lat, lon)
	case "/forecast/hour":
		data, err = h.opts.Weather.Hourly(ctx, lat, lon, req.QueryInt("duration", 13))
	case "/forecast/day":
		data, err = h.opts.Weather.Daily(ctx, lat, lon, req.QueryInt("duration", 5))
	case "/pointWinds":
		// Only the optional extension of WeatherSource can answer this one.
		src, ok := h.opts.Weather.(PointWindsSource)
		if !ok {
			h.log.Warn("garminhttp: pointWinds requested but source has no wind data")
			return nil
		}
		if format := req.Query.Get("rspFmt"); format != "" && format != "json" {
			h.log.Warn("garminhttp: unknown pointWinds response format", "rspFmt", format)
			return nil
		}
		data, err = src.PointWinds(ctx, lat, lon)
	default:
		h.log.Warn("garminhttp: unknown weather path", "path", path)
		return nil
	}
	if err != nil {
		h.log.Warn("garminhttp: weather source failed", "path", path, "err", err)
		return nil
	}

	body, err := json.Marshal(data)
	if err != nil {
		h.log.Error("garminhttp: marshal weather", "err", err)
		return nil
	}

	h.log.Debug("garminhttp: weather reply", "path", path, "version", version, "bytes", len(body))
	resp := newResponse(200)
	resp.Body = body
	resp.SetHeader("Content-Type", "application/json")
	return resp
}

func notFound() *Response {
	resp := newResponse(404)
	resp.Body = []byte("{}")
	resp.SetHeader("Content-Type", "application/json")
	return resp
}

// weatherCoords normalises the lat/lon query parameters the watch sticks on
// its GCS weather URLs. Real firmware sends Garmin semicircles (values on the
// order of 1e8–1e9); unit tests and older firmware send plain degrees. Anything
// outside the geographic range is treated as semicircles.
func weatherCoords(lat, lon float64) (float64, float64) {
	const scale = 180.0 / 2147483648.0
	if lat < -90 || lat > 90 {
		lat *= scale
	}
	if lon < -180 || lon > 180 {
		lon *= scale
	}
	return lat, lon
}
