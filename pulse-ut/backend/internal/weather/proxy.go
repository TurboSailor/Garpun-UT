package weather

import (
	"context"
	"math"
)

// Units the watch asks for by default on the endpoints it polls
// (http/interceptors/WeatherInterceptor.java:96-136).
const (
	unitCelsius         = "CELSIUS"
	unitMetersPerSec    = "METERS_PER_SECOND"
	unitKmPerHour       = "KILOMETERS_PER_HOUR"
	unitMeter           = "METER"
	unitMillibar        = "MILLIBAR"
	unitInchesOfMercury = "INCHES_OF_MERCURY"

	defaultDays  = 5
	defaultHours = 13
)

// Value mirrors WeatherInterceptor.WeatherValue.
type Value struct {
	Value float64 `json:"value"`
	Units string  `json:"units"`
}

// Wind mirrors WeatherInterceptor.Wind.
type Wind struct {
	Speed           *Value `json:"speed,omitempty"`
	DirectionString string `json:"directionString,omitempty"`
	Direction       *int   `json:"direction,omitempty"`
}

// Current mirrors WeatherInterceptor.WeatherForecastCurrent.
type Current struct {
	EpochSeconds         int64  `json:"epochSeconds"`
	Temperature          *Value `json:"temperature,omitempty"`
	Description          string `json:"description,omitempty"`
	Icon                 *int   `json:"icon,omitempty"`
	FeelsLikeTemperature *Value `json:"feelsLikeTemperature,omitempty"`
	DewPoint             *Value `json:"dewPoint,omitempty"`
	RelativeHumidity     *int   `json:"relativeHumidity,omitempty"`
	Wind                 *Wind  `json:"wind,omitempty"`
	LocationName         string `json:"locationName,omitempty"`
	Visibility           *Value `json:"visibility,omitempty"`
	Pressure             *Value `json:"pressure,omitempty"`
	PressureChange       *Value `json:"pressureChange,omitempty"`
	CloudCoverage        *int   `json:"cloudCoverage,omitempty"`
}

// Hour mirrors WeatherInterceptor.WeatherForecastHour.
type Hour struct {
	EpochSeconds         int64    `json:"epochSeconds"`
	Description          string   `json:"description,omitempty"`
	Temp                 *Value   `json:"temp,omitempty"`
	PrecipProb           *int     `json:"precipProb,omitempty"`
	Wind                 *Wind    `json:"wind,omitempty"`
	Icon                 *int     `json:"icon,omitempty"`
	DewPoint             *Value   `json:"dewPoint,omitempty"`
	UvIndex              *float64 `json:"uvIndex,omitempty"`
	RelativeHumidity     *int     `json:"relativeHumidity,omitempty"`
	FeelsLikeTemperature *Value   `json:"feelsLikeTemperature,omitempty"`
	Visibility           *Value   `json:"visibility,omitempty"`
	Pressure             *Value   `json:"pressure,omitempty"`
	AirQuality           *int     `json:"airQuality,omitempty"`
	CloudCover           *int     `json:"cloudCover,omitempty"`
}

// Day mirrors WeatherInterceptor.WeatherForecastDay. dayOfWeek follows the v2
// convention: 1 is Monday, 7 is Sunday.
type Day struct {
	DayOfWeek    int    `json:"dayOfWeek"`
	Description  string `json:"description,omitempty"`
	Summary      string `json:"summary,omitempty"`
	High         *Value `json:"high,omitempty"`
	Low          *Value `json:"low,omitempty"`
	PrecipProb   *int   `json:"precipProb,omitempty"`
	Icon         *int   `json:"icon,omitempty"`
	EpochSunrise *int64 `json:"epochSunrise,omitempty"`
	EpochSunset  *int64 `json:"epochSunset,omitempty"`
	Wind         *Wind  `json:"wind,omitempty"`
	Humidity     *int   `json:"humidity,omitempty"`
}

// PointWindsResponse mirrors WeatherInterceptor.PointWindsResponse.
type PointWindsResponse struct {
	CcPointWinds CcPointWinds `json:"CcPointWinds"`
}

// CcPointWinds mirrors WeatherInterceptor.CcPointWinds.
type CcPointWinds struct {
	I   int64       `json:"i"`
	Lat float64     `json:"lat"`
	Lon float64     `json:"lon"`
	W   []PointWind `json:"W"`
}

// PointWind mirrors WeatherInterceptor.PointWind: offset seconds, knots,
// degrees and gusts in knots.
type PointWind struct {
	T int64   `json:"t"`
	S float64 `json:"s"`
	D int     `json:"d"`
	G float64 `json:"g"`
}

// Current returns the object served on /weather/v{1,2}/current.
func (s *Service) Current(ctx context.Context, lat, lon float64) (any, error) {
	sp, err := s.forecast(ctx, lat, lon)
	if err != nil {
		return nil, err
	}
	out := &Current{
		EpochSeconds:         sp.timestamp,
		Temperature:          temperature(sp.temp),
		Description:          sp.condition,
		Icon:                 new(owmToGarminIcon(sp.conditionCode)),
		FeelsLikeTemperature: temperature(sp.feelsLike),
		DewPoint:             temperature(sp.dewPoint),
		RelativeHumidity:     percentPtr(sp.humidity),
		Wind:                 wind(sp.windSpeed, sp.windDirection, unitMetersPerSec),
		LocationName:         sp.location,
		PressureChange:       &Value{Value: 0, Units: unitInchesOfMercury},
		CloudCoverage:        percentPtr(sp.cloudCover),
	}
	if sp.visibility > 0 {
		out.Visibility = &Value{Value: sp.visibility, Units: unitMeter}
	}
	if sp.pressure > 0 {
		out.Pressure = &Value{Value: sp.pressure * 0.02953, Units: unitInchesOfMercury}
	}
	return out, nil
}

// Hourly returns the array served on /weather/v{1,2}/forecast/hour.
func (s *Service) Hourly(ctx context.Context, lat, lon float64, hours int) (any, error) {
	sp, err := s.forecast(ctx, lat, lon)
	if err != nil {
		return nil, err
	}
	if hours <= 0 {
		hours = defaultHours
	}
	out := make([]Hour, 0, min(hours, len(sp.hourly)))
	for i := range min(hours, len(sp.hourly)) {
		h := &sp.hourly[i]
		hour := Hour{
			EpochSeconds:         h.timestamp,
			Description:          h.condition,
			Temp:                 temperature(h.temp),
			PrecipProb:           percentPtr(h.precipProb),
			Wind:                 wind(h.windSpeed, h.windDirection, unitMetersPerSec),
			Icon:                 new(owmToGarminIcon(h.conditionCode)),
			DewPoint:             temperature(h.dewPoint),
			RelativeHumidity:     percentPtr(h.humidity),
			FeelsLikeTemperature: temperature(h.feelsLike),
			CloudCover:           percentPtr(h.cloudCover),
		}
		if h.uvIndex >= 0 {
			hour.UvIndex = new(h.uvIndex)
		}
		if h.visibility >= 0 {
			hour.Visibility = &Value{Value: h.visibility, Units: unitMeter}
		}
		if h.pressure > 0 {
			hour.Pressure = &Value{Value: h.pressure, Units: unitMillibar}
		}
		out = append(out, hour)
	}
	return out, nil
}

// Daily returns the array served on /weather/v{1,2}/forecast/day.
func (s *Service) Daily(ctx context.Context, lat, lon float64, days int) (any, error) {
	sp, err := s.forecast(ctx, lat, lon)
	if err != nil {
		return nil, err
	}
	if days <= 0 {
		days = defaultDays
	}
	out := make([]Day, 0, min(days, len(sp.daily)))
	for i := range min(days, len(sp.daily)) {
		d := &sp.daily[i]
		day := Day{
			DayOfWeek:   isoDayOfWeek(d.timestamp, sp.utcOffset),
			Description: d.condition,
			Summary:     d.condition,
			High:        temperature(d.maxTemp),
			Low:         temperature(d.minTemp),
			PrecipProb:  percentPtr(d.precipProb),
			Icon:        new(owmToGarminIcon(d.conditionCode)),
			// Day wind is reported in km/h, the v2 default for this endpoint.
			Wind:     wind(d.windSpeed, d.windDirection, unitKmPerHour),
			Humidity: percentPtr(d.humidity),
		}
		if d.sunrise > 0 && d.sunset > 0 {
			day.EpochSunrise, day.EpochSunset = new(d.sunrise), new(d.sunset)
		}
		out = append(out, day)
	}
	return out, nil
}

// PointWinds returns the object served on /weather/pointWinds: up to four wind
// samples relative to the first forecast hour.
func (s *Service) PointWinds(ctx context.Context, lat, lon float64) (any, error) {
	sp, err := s.forecast(ctx, lat, lon)
	if err != nil {
		return nil, err
	}
	out := &PointWindsResponse{CcPointWinds: CcPointWinds{
		I:   sp.timestamp,
		Lat: lat,
		Lon: lon,
		W:   []PointWind{},
	}}
	if len(sp.hourly) == 0 {
		return out, nil
	}
	base := sp.hourly[0].timestamp
	for i := range min(4, len(sp.hourly)) {
		h := &sp.hourly[i]
		knots := 0.0
		if h.windSpeed > 0 {
			knots = h.windSpeed / 1.852
		}
		direction := h.windDirection
		if direction < 0 {
			direction = 0
		}
		out.CcPointWinds.W = append(out.CcPointWinds.W, PointWind{
			T: h.timestamp - base,
			S: knots,
			D: direction,
			G: knots * 1.47,
		})
	}
	return out, nil
}

// temperature applies the same deliberately "wrong" kelvin to celsius shift as
// the FIT path (issue #4313).
func temperature(kelvin int) *Value {
	if kelvin <= 0 {
		return nil
	}
	return &Value{Value: float64(kelvin - 273), Units: unitCelsius}
}

func wind(kmh float64, degrees int, unit string) *Wind {
	if kmh < 0 && degrees < 0 {
		return nil
	}
	w := &Wind{}
	if kmh >= 0 {
		value := kmh
		if unit == unitMetersPerSec {
			value = kmh / 3.6
		}
		w.Speed = &Value{Value: value, Units: unit}
	}
	if degrees >= 0 {
		w.DirectionString = compassPoint(degrees)
		w.Direction = new(degrees)
	}
	return w
}

// compassPoint names one of eight rhumbs, as WeatherInterceptor.getWindDirection.
func compassPoint(degrees int) string {
	degrees = ((degrees % 360) + 360) % 360
	points := [...]string{"N", "NE", "E", "SE", "S", "SW", "W", "NW"}
	return points[int(math.Round(float64(degrees)/45))%8]
}

// isoDayOfWeek is the v2 convention: 1 Monday .. 7 Sunday, named in the
// forecast location's calendar.
func isoDayOfWeek(unix int64, utcOffset int) int {
	if wd := int(weekdayAt(unix, utcOffset)); wd != 0 {
		return wd
	}
	return 7
}

func percentPtr(v int) *int {
	if v < 0 {
		return nil
	}
	return &v
}
