package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

// Open-Meteo variable lists. Same as util/PulseWeather.java plus the series the
// upstream FIT/JSON encoders declare but never filled: hourly apparent
// temperature, pressure and cloud cover, and daily wind.
const (
	currentVars = "temperature_2m,relative_humidity_2m,apparent_temperature,weather_code," +
		"wind_speed_10m,wind_direction_10m,surface_pressure,cloud_cover,dew_point_2m"
	hourlyVars = "temperature_2m,apparent_temperature,weather_code,relative_humidity_2m," +
		"wind_speed_10m,wind_direction_10m,precipitation_probability,uv_index,dew_point_2m," +
		"visibility,surface_pressure,cloud_cover"
	dailyVars = "weather_code,temperature_2m_max,temperature_2m_min,sunrise,sunset," +
		"uv_index_max,precipitation_probability_max,wind_speed_10m_max,wind_direction_10m_dominant"

	// hourlyWindow is how many hours ahead we keep, matching upstream.
	hourlyWindow = 24
	// forecastDays is what Open-Meteo is asked for; today plus six.
	forecastDays = 7
)

// nums is an Open-Meteo series. Entries are null when the model has no value
// for that step, which happens at the edges of the horizon.
type nums []*float64

func (n nums) at(i int) (float64, bool) {
	if i < 0 || i >= len(n) || n[i] == nil {
		return 0, false
	}
	return *n[i], true
}

func (n nums) intAt(i int) (int, bool) {
	f, ok := n.at(i)
	if !ok {
		return 0, false
	}
	return int(math.Round(f)), ok
}

// intOr returns the reading or the sentinel upstream treats as "no data".
func (n nums) intOr(i, missing int) int {
	if v, ok := n.intAt(i); ok {
		return v
	}
	return missing
}

func (n nums) floatOr(i int, missing float64) float64 {
	if v, ok := n.at(i); ok {
		return v
	}
	return missing
}

type omResponse struct {
	Timezone  string `json:"timezone"`
	UTCOffset int    `json:"utc_offset_seconds"`

	Current struct {
		Time                int64    `json:"time"`
		Temperature         float64  `json:"temperature_2m"`
		RelativeHumidity    *int     `json:"relative_humidity_2m"`
		ApparentTemperature *float64 `json:"apparent_temperature"`
		WeatherCode         *int     `json:"weather_code"`
		WindSpeed           *float64 `json:"wind_speed_10m"`
		WindDirection       *int     `json:"wind_direction_10m"`
		SurfacePressure     *float64 `json:"surface_pressure"`
		CloudCover          *int     `json:"cloud_cover"`
		DewPoint            *float64 `json:"dew_point_2m"`
	} `json:"current"`

	Hourly struct {
		Time                []int64 `json:"time"`
		Temperature         nums    `json:"temperature_2m"`
		ApparentTemperature nums    `json:"apparent_temperature"`
		WeatherCode         nums    `json:"weather_code"`
		RelativeHumidity    nums    `json:"relative_humidity_2m"`
		WindSpeed           nums    `json:"wind_speed_10m"`
		WindDirection       nums    `json:"wind_direction_10m"`
		PrecipProbability   nums    `json:"precipitation_probability"`
		UVIndex             nums    `json:"uv_index"`
		DewPoint            nums    `json:"dew_point_2m"`
		Visibility          nums    `json:"visibility"`
		SurfacePressure     nums    `json:"surface_pressure"`
		CloudCover          nums    `json:"cloud_cover"`
	} `json:"hourly"`

	Daily struct {
		Time              []int64 `json:"time"`
		WeatherCode       nums    `json:"weather_code"`
		TemperatureMax    nums    `json:"temperature_2m_max"`
		TemperatureMin    nums    `json:"temperature_2m_min"`
		Sunrise           []int64 `json:"sunrise"`
		Sunset            []int64 `json:"sunset"`
		UVIndexMax        nums    `json:"uv_index_max"`
		PrecipProbability nums    `json:"precipitation_probability_max"`
		WindSpeedMax      nums    `json:"wind_speed_10m_max"`
		WindDirection     nums    `json:"wind_direction_10m_dominant"`
	} `json:"daily"`
}

func (s *Service) fetch(ctx context.Context, lat, lon float64) (*omResponse, error) {
	// wind_speed_unit=kmh: WeatherSpec and the FIT encoder both expect km/h.
	// Upstream asks for m/s here and then divides by 3.6 again, which
	// under-reports wind on the watch by that factor.
	url := fmt.Sprintf("%s?latitude=%.4f&longitude=%.4f&current=%s&hourly=%s&daily=%s"+
		"&timezone=auto&timeformat=unixtime&wind_speed_unit=kmh&forecast_days=%d",
		s.base, lat, lon, currentVars, hourlyVars, dailyVars, forecastDays)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("weather: request: %w", err)
	}
	req.Header.Set("User-Agent", s.ua)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("weather: open-meteo: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Open-Meteo reports the offending parameter in the body; keep a bit.
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("weather: open-meteo http %d: %s", resp.StatusCode,
			strings.TrimSpace(string(snippet)))
	}

	var out omResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("weather: decode: %w", err)
	}
	return &out, nil
}

func (s *Service) buildSpec(r *omResponse, lat, lon float64) *spec {
	now := time.Now().Unix()
	sp := &spec{
		timestamp:  now,
		observedAt: now,
		lat:        lat,
		lon:        lon,
		location:   s.name,
		utcOffset:  r.UTCOffset,

		humidity:      -1,
		windSpeed:     -1,
		windDirection: -1,
		cloudCover:    -1,
		uvIndex:       -1,
		precipProb:    -1,
	}
	if sp.location == "" {
		sp.location = locationFromTimezone(r.Timezone)
	}
	if r.Current.Time > 0 {
		sp.observedAt = r.Current.Time
	}

	cur := &r.Current
	sp.temp = toKelvin(cur.Temperature, true)
	sp.feelsLike = sp.temp
	if cur.ApparentTemperature != nil {
		sp.feelsLike = toKelvin(*cur.ApparentTemperature, true)
	}
	sp.dewPoint = sp.temp
	if cur.DewPoint != nil {
		sp.dewPoint = toKelvin(*cur.DewPoint, true)
	}
	if cur.RelativeHumidity != nil {
		sp.humidity = *cur.RelativeHumidity
	}
	if cur.WindSpeed != nil {
		sp.windSpeed = *cur.WindSpeed
	}
	if cur.WindDirection != nil {
		sp.windDirection = *cur.WindDirection
	}
	if cur.SurfacePressure != nil {
		sp.pressure = *cur.SurfacePressure
	}
	if cur.CloudCover != nil {
		sp.cloudCover = *cur.CloudCover
	}
	wmo := 0
	if cur.WeatherCode != nil {
		wmo = *cur.WeatherCode
	}
	sp.conditionCode = wmoToOwm(wmo)
	sp.condition = wmoText(wmo)

	s.buildDaily(sp, r)
	s.buildHourly(sp, r, now)
	return sp
}

func (s *Service) buildDaily(sp *spec, r *omResponse) {
	d := &r.Daily
	if len(d.Time) == 0 {
		return
	}
	sp.daily = make([]daySpec, 0, len(d.Time))
	for i := range d.Time {
		wmo := d.WeatherCode.intOr(i, 0)
		day := daySpec{
			timestamp:     d.Time[i],
			minTemp:       toKelvinAt(d.TemperatureMin, i),
			maxTemp:       toKelvinAt(d.TemperatureMax, i),
			conditionCode: wmoToOwm(wmo),
			condition:     wmoText(wmo),
			uvIndex:       d.UVIndexMax.floatOr(i, -1),
			precipProb:    d.PrecipProbability.intOr(i, -1),
			windSpeed:     d.WindSpeedMax.floatOr(i, -1),
			windDirection: d.WindDirection.intOr(i, -1),
			// Open-Meteo has no daily humidity or pressure aggregate.
			humidity: -1,
		}
		if i < len(d.Sunrise) {
			day.sunrise = d.Sunrise[i]
		}
		if i < len(d.Sunset) {
			day.sunset = d.Sunset[i]
		}
		sp.daily = append(sp.daily, day)
	}
	today := sp.daily[0]
	sp.todayMin, sp.todayMax = today.minTemp, today.maxTemp
	sp.uvIndex, sp.precipProb = today.uvIndex, today.precipProb
}

func (s *Service) buildHourly(sp *spec, r *omResponse, now int64) {
	h := &r.Hourly
	if len(h.Time) == 0 {
		return
	}
	// Upstream starts at the last hour that has not fully elapsed.
	start := 0
	for i := range h.Time {
		if h.Time[i] >= now-3600 {
			start = i
			break
		}
	}
	// Open-Meteo reports no "current" visibility, so borrow the hour's.
	sp.visibility = h.Visibility.floatOr(start, 10000)

	sp.hourly = make([]hourSpec, 0, hourlyWindow)
	for i := start; i < len(h.Time) && len(sp.hourly) < hourlyWindow; i++ {
		wmo := h.WeatherCode.intOr(i, 0)
		temp := toKelvinAt(h.Temperature, i)
		feels := toKelvinAt(h.ApparentTemperature, i)
		if feels == 0 {
			feels = temp
		}
		sp.hourly = append(sp.hourly, hourSpec{
			timestamp:     h.Time[i],
			temp:          temp,
			feelsLike:     feels,
			dewPoint:      toKelvinAt(h.DewPoint, i),
			conditionCode: wmoToOwm(wmo),
			condition:     wmoText(wmo),
			humidity:      h.RelativeHumidity.intOr(i, -1),
			windSpeed:     h.WindSpeed.floatOr(i, -1),
			windDirection: h.WindDirection.intOr(i, -1),
			precipProb:    h.PrecipProbability.intOr(i, -1),
			uvIndex:       h.UVIndex.floatOr(i, -1),
			pressure:      h.SurfacePressure.floatOr(i, 0),
			visibility:    h.Visibility.floatOr(i, -1),
			cloudCover:    h.CloudCover.intOr(i, -1),
		})
	}
}

func toKelvinAt(series nums, i int) int {
	f, ok := series.at(i)
	return toKelvin(f, ok)
}
