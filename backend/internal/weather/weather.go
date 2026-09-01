// Package weather fetches forecasts from Open-Meteo and renders them in the two
// shapes a Garmin watch understands: FIT WEATHER records pushed over GFDI, and
// the JSON the watch pulls through the local HTTP proxy.
package weather

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Defaults mirroring util/PulseWeather.java: one fetch per quarter hour, no API
// key, plain HTTP GET.
const (
	defaultBaseURL   = "https://api.open-meteo.com/v1/forecast"
	defaultUserAgent = "Pulse/1.0"
	defaultTTL       = 15 * time.Minute
	defaultTimeout   = 15 * time.Second
)

// Options configures the service; every field is optional.
type Options struct {
	// BaseURL overrides the Open-Meteo forecast endpoint.
	BaseURL string
	// UserAgent sent with every request.
	UserAgent string
	// Client overrides the HTTP client, including its timeout.
	Client *http.Client
	// TTL is how long a fetched forecast is reused.
	TTL time.Duration
	// LocationName overrides the name shown on the watch. Empty derives it
	// from the timezone Open-Meteo reports, since Open-Meteo has no reverse
	// geocoder and Ubuntu Touch has no Android Geocoder equivalent.
	LocationName string
}

// Service is the cached Open-Meteo client.
type Service struct {
	log    *slog.Logger
	client *http.Client
	base   string
	ua     string
	ttl    time.Duration
	name   string

	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	at   time.Time
	spec *spec
}

// New builds a service. log may be nil.
func New(log *slog.Logger, opts Options) *Service {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	s := &Service{
		log:    log,
		client: opts.Client,
		base:   opts.BaseURL,
		ua:     opts.UserAgent,
		ttl:    opts.TTL,
		name:   opts.LocationName,
		cache:  map[string]cached{},
	}
	if s.client == nil {
		s.client = &http.Client{Timeout: defaultTimeout}
	}
	if s.base == "" {
		s.base = defaultBaseURL
	}
	if s.ua == "" {
		s.ua = defaultUserAgent
	}
	if s.ttl <= 0 {
		s.ttl = defaultTTL
	}
	return s
}

// SemicirclesToDegrees converts the coordinates the watch sends in
// WEATHER_REQUEST (5014) to degrees.
func SemicirclesToDegrees(v int32) float64 {
	return float64(v) * (180.0 / 2147483648.0)
}

// forecast returns a cached or freshly fetched forecast for the coordinates.
func (s *Service) forecast(ctx context.Context, lat, lon float64) (*spec, error) {
	// The request itself rounds to four decimals, so cache at that precision.
	key := fmt.Sprintf("%.4f,%.4f", lat, lon)

	s.mu.Lock()
	entry, ok := s.cache[key]
	s.mu.Unlock()
	if ok && time.Since(entry.at) < s.ttl {
		return entry.spec, nil
	}

	raw, err := s.fetch(ctx, lat, lon)
	if err != nil {
		// A stale forecast beats no forecast at all when the network is down.
		if ok {
			s.log.Warn("weather: reusing stale forecast", "err", err, "age", time.Since(entry.at))
			return entry.spec, nil
		}
		return nil, err
	}
	sp := s.buildSpec(raw, lat, lon)

	s.mu.Lock()
	s.cache[key] = cached{at: time.Now(), spec: sp}
	s.mu.Unlock()
	return sp, nil
}

// spec is the normalized forecast both output paths consume. It follows
// WeatherSpec upstream: temperatures in kelvin with 0 meaning unknown, wind in
// km/h, pressure in millibar, and negative percentages/directions meaning
// "no data" — the exact convention FitWeather.Builder checks for.
type spec struct {
	timestamp  int64
	observedAt int64
	lat, lon   float64
	location   string
	// utcOffset is the forecast location's offset, so weekdays are named in
	// its calendar rather than the daemon host's.
	utcOffset int

	temp          int
	feelsLike     int
	dewPoint      int
	humidity      int
	windSpeed     float64
	windDirection int
	pressure      float64
	cloudCover    int
	visibility    float64
	conditionCode int
	condition     string
	uvIndex       float64
	precipProb    int
	todayMin      int
	todayMax      int

	hourly []hourSpec
	daily  []daySpec
}

type hourSpec struct {
	timestamp     int64
	temp          int
	feelsLike     int
	dewPoint      int
	conditionCode int
	condition     string
	humidity      int
	windSpeed     float64
	windDirection int
	precipProb    int
	uvIndex       float64
	pressure      float64
	visibility    float64
	cloudCover    int
}

type daySpec struct {
	timestamp     int64
	minTemp       int
	maxTemp       int
	conditionCode int
	condition     string
	sunrise       int64
	sunset        int64
	uvIndex       float64
	precipProb    int
	windSpeed     float64
	windDirection int
	humidity      int
	pressure      float64
}

// toKelvin matches PulseWeather.toKelvin; 0 marks a missing reading.
func toKelvin(celsius float64, ok bool) int {
	if !ok {
		return 0
	}
	return int(math.Round(celsius + 273.15))
}

// locationFromTimezone turns "Europe/Moscow" into "Moscow".
func locationFromTimezone(tz string) string {
	if i := strings.LastIndexByte(tz, '/'); i >= 0 {
		tz = tz[i+1:]
	}
	return strings.ReplaceAll(tz, "_", " ")
}
