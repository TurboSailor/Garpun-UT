package uxbridge

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestParseMPRISMetadata(t *testing.T) {
	md := map[string]dbus.Variant{
		"mpris:trackid": dbus.MakeVariant(dbus.ObjectPath("/track/1")),
		"mpris:length":  dbus.MakeVariant(int64(245_000_000)),
		"xesam:title":   dbus.MakeVariant("Attack"),
		"xesam:artist":  dbus.MakeVariant([]string{"Thirty Seconds to Mars", "Guest"}),
		"xesam:album":   dbus.MakeVariant("A Beautiful Lie"),
	}
	got := parseMPRISMetadata(md)
	want := mprisTrack{
		Title:    "Attack",
		Artist:   "Thirty Seconds to Mars, Guest",
		Album:    "A Beautiful Lie",
		Duration: 245,
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestParseMPRISMetadataLooseTypes(t *testing.T) {
	// Players are sloppy: a bare artist string and a uint64 length are common.
	md := map[string]dbus.Variant{
		"xesam:artist": dbus.MakeVariant("Solo"),
		"mpris:length": dbus.MakeVariant(uint64(90_500_000)),
	}
	got := parseMPRISMetadata(md)
	if got.Artist != "Solo" {
		t.Errorf("artist = %q", got.Artist)
	}
	if got.Duration != 90 {
		t.Errorf("duration = %d, want 90", got.Duration)
	}

	if got := parseMPRISMetadata(nil); got != (mprisTrack{}) {
		t.Errorf("nil metadata = %+v", got)
	}
}
