package garminhttp

import (
	"math"
	"testing"
)

func TestWeatherCoordsDegreesPassthrough(t *testing.T) {
	lat, lon := weatherCoords(55.75, 37.62)
	if math.Abs(lat-55.75) > 1e-9 || math.Abs(lon-37.62) > 1e-9 {
		t.Fatalf("got %v,%v", lat, lon)
	}
}

func TestWeatherCoordsSemicircles(t *testing.T) {
	// Live Forerunner 255 request: lat=717048945 lon=372792160.
	lat, lon := weatherCoords(717048945, 372792160)
	wantLat := 717048945 * (180.0 / 2147483648.0)
	wantLon := 372792160 * (180.0 / 2147483648.0)
	if math.Abs(lat-wantLat) > 1e-9 || math.Abs(lon-wantLon) > 1e-9 {
		t.Fatalf("got %v,%v want %v,%v", lat, lon, wantLat, wantLon)
	}
	if lat < 50 || lat > 70 || lon < 20 || lon > 50 {
		t.Fatalf("converted coords outside Moscow-ish range: %v,%v", lat, lon)
	}
}

func TestWeatherCoordsNegativeHemisphere(t *testing.T) {
	lat, lon := weatherCoords(-665192604, -1224190000)
	if lat >= 0 || lon >= 0 {
		t.Fatalf("expected southern/western coords, got %v,%v", lat, lon)
	}
}
