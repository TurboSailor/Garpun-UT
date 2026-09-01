package weather

import (
	"context"
	"fmt"
	"math"
	"time"

	"pulse/backend/internal/fit"
)

// Record counts the watch expects, from GarminSupport.encodeWeather.
const (
	defaultForecastHours = 12
	maxForecastHours     = 24
	forecastDayRecords   = 5 // today plus four
)

// FitPayload builds the WEATHER (global message 128) record stream pushed to
// the watch after a WEATHER_REQUEST: one current report, up to hours of hourly
// forecast, then today plus four daily forecasts. Feed it straight to
// garmin.Session.SendFitRecords; definitions lead the stream, so
// fit.SplitRecords finds the FIT_DEFINITION / FIT_DATA boundary.
func (s *Service) FitPayload(ctx context.Context, lat, lon float64, hours int) ([]byte, error) {
	sp, err := s.forecast(ctx, lat, lon)
	if err != nil {
		return nil, err
	}
	if hours <= 0 {
		hours = defaultForecastHours
	}
	if hours > maxForecastHours {
		hours = maxForecastHours
	}

	b := fit.NewBuilder()
	if err := b.Add("WEATHER", map[string]any{
		"weather_report":            reportCurrent,
		"timestamp":                 sp.timestamp,
		"observed_at_time":          sp.observedAt,
		"temperature":               fitTemp(sp.temp),
		"low_temperature":           fitTemp(sp.todayMin),
		"high_temperature":          fitTemp(sp.todayMax),
		"condition":                 owmToFitCondition(sp.conditionCode),
		"wind_direction":            fitWindDirection(sp.windDirection),
		"precipitation_probability": fitPercent(sp.precipProb),
		"wind_speed":                fitWindSpeed(sp.windSpeed),
		"temperature_feels_like":    fitTemp(sp.feelsLike),
		"relative_humidity":         fitPercent(sp.humidity),
		"observed_location_lat":     sp.lat,
		"observed_location_long":    sp.lon,
		// Open-Meteo's free forecast API carries no AQI; the field is still
		// declared so the watch stops asking for it.
		"air_quality": nil,
		"dew_point":   fitTemp(sp.dewPoint),
		"location":    sp.location,
	}); err != nil {
		return nil, fmt.Errorf("weather: current record: %w", err)
	}

	// Every hourly record must carry the same field set: they share one local
	// message type, so nil stands in for missing readings.
	for i := range min(hours, len(sp.hourly)) {
		h := &sp.hourly[i]
		if err := b.Add("WEATHER", map[string]any{
			"weather_report":            reportHourly,
			"timestamp":                 h.timestamp,
			"temperature":               fitTemp(h.temp),
			"condition":                 owmToFitCondition(h.conditionCode),
			"wind_direction":            fitWindDirection(h.windDirection),
			"wind_speed":                fitWindSpeed(h.windSpeed),
			"precipitation_probability": fitPercent(h.precipProb),
			"temperature_feels_like":    fitTemp(h.feelsLike),
			"relative_humidity":         fitPercent(h.humidity),
			"dew_point":                 fitTemp(h.dewPoint),
			"uv_index":                  fitUVIndex(h.uvIndex),
			"air_quality":               nil,
			"atmospheric_pressure":      fitPressure(h.pressure),
		}); err != nil {
			return nil, fmt.Errorf("weather: hourly record %d: %w", i, err)
		}
	}

	for i := range min(forecastDayRecords, len(sp.daily)) {
		d := &sp.daily[i]
		if err := b.Add("WEATHER", map[string]any{
			"weather_report":            reportDaily,
			"timestamp":                 d.timestamp,
			"low_temperature":           fitTemp(d.minTemp),
			"high_temperature":          fitTemp(d.maxTemp),
			"condition":                 owmToFitCondition(d.conditionCode),
			"precipitation_probability": fitPercent(d.precipProb),
			"day_of_week":               fitDayOfWeek(d.timestamp, sp.utcOffset),
			"air_quality":               nil,
			"relative_humidity":         fitPercent(d.humidity),
			"wind_speed":                fitWindSpeed(d.windSpeed),
			"wind_direction":            fitWindDirection(d.windDirection),
			"uv_index":                  fitUVIndex(d.uvIndex),
			"atmospheric_pressure":      fitPressure(d.pressure),
		}); err != nil {
			return nil, fmt.Errorf("weather: daily record %d: %w", i, err)
		}
	}

	return b.Records(), nil
}

// The conversions below mirror fit/messages/FitWeather.java:37-170 one for one,
// including the deliberately "wrong" kelvin to celsius shift (issue #4313).

func fitTemp(kelvin int) any {
	if kelvin > 0 {
		return kelvin - 273
	}
	return nil
}

func fitWindSpeed(kmh float64) any {
	if kmh < 0 {
		return nil
	}
	speed := int(math.Round(kmh / 3.6 * 1000))
	if speed >= 0xFFFF {
		speed = 0xFFFE
	}
	return speed
}

func fitWindDirection(degrees int) any {
	if degrees < 0 {
		return nil
	}
	return degrees % 360
}

func fitPercent(v int) any {
	if v < 0 {
		return nil
	}
	if v > 100 {
		v = 100
	}
	return v
}

// fitUVIndex clamps instead of rescaling: 10 and above is extreme exposure in
// both the WeatherSpec (0..15) and FIT (0..10) scales.
func fitUVIndex(uv float64) any {
	if uv < 0 {
		return nil
	}
	if uv > 10 {
		uv = 10
	}
	return uv
}

func fitPressure(millibar float64) any {
	if millibar <= 0 {
		return nil
	}
	return int64(math.Round(millibar * 100))
}

// fitDayOfWeek encodes the weekday the way FieldDefinitionDayOfWeek.encode
// does: DayOfWeek.getValue() % 7, so Sunday is 0. Upstream reads the host
// timezone; utcOffset is the forecast location's, which is what the watch
// displays even when the two differ.
func fitDayOfWeek(unix int64, utcOffset int) any {
	return int(weekdayAt(unix, utcOffset))
}

func weekdayAt(unix int64, utcOffset int) time.Weekday {
	return time.Unix(unix+int64(utcOffset), 0).UTC().Weekday()
}
