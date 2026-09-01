package uxbridge

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

// Lomiri pushes volume and brightness bars through the same Notify call as
// real notifications. Forwarding those buzzes the watch on every keypress,
// which is what this filter exists to prevent.
func TestIsSystemOverlay(t *testing.T) {
	overlays := []struct {
		name  string
		hints map[string]dbus.Variant
	}{
		{"volume bar", map[string]dbus.Variant{
			"x-canonical-private-synchronous": dbus.MakeVariant("volume"),
			"value":                           dbus.MakeVariant(int32(40)),
		}},
		{"lomiri synchronous", map[string]dbus.Variant{
			"x-lomiri-private-synchronous": dbus.MakeVariant("brightness"),
		}},
		{"icon only", map[string]dbus.Variant{
			"x-lomiri-private-icon-only": dbus.MakeVariant(true),
		}},
		{"transient bool", map[string]dbus.Variant{
			"transient": dbus.MakeVariant(true),
		}},
		{"transient as byte", map[string]dbus.Variant{
			"transient": dbus.MakeVariant(uint8(1)),
		}},
		{"bare progress value", map[string]dbus.Variant{
			"value": dbus.MakeVariant(int32(70)),
		}},
	}
	for _, c := range overlays {
		if _, got := isSystemOverlay(c.hints); !got {
			t.Errorf("%s: expected it to be treated as an overlay", c.name)
		}
	}

	real := []struct {
		name  string
		hints map[string]dbus.Variant
	}{
		{"plain message", map[string]dbus.Variant{}},
		{"app with desktop entry", map[string]dbus.Variant{
			"desktop-entry": dbus.MakeVariant("telegram"),
		}},
		{"urgent message", map[string]dbus.Variant{
			"urgency": dbus.MakeVariant(uint8(2)),
		}},
		// transient explicitly false must not be filtered.
		{"transient false", map[string]dbus.Variant{
			"transient": dbus.MakeVariant(false),
		}},
	}
	for _, c := range real {
		if marker, got := isSystemOverlay(c.hints); got {
			t.Errorf("%s: wrongly treated as an overlay via %q", c.name, marker)
		}
	}
}

// The marker is reported so the log says which hint caused the drop.
func TestSystemOverlayNamesTheMarker(t *testing.T) {
	marker, ok := isSystemOverlay(map[string]dbus.Variant{
		"x-canonical-private-synchronous": dbus.MakeVariant("volume"),
	})
	if !ok {
		t.Fatal("expected an overlay")
	}
	if marker != "x-canonical-private-synchronous" {
		t.Errorf("marker = %q", marker)
	}
}

func TestTruthyReadsNumericHints(t *testing.T) {
	cases := map[string]struct {
		v    dbus.Variant
		want bool
	}{
		"bool true":   {dbus.MakeVariant(true), true},
		"bool false":  {dbus.MakeVariant(false), false},
		"byte 1":      {dbus.MakeVariant(uint8(1)), true},
		"byte 0":      {dbus.MakeVariant(uint8(0)), false},
		"int32 1":     {dbus.MakeVariant(int32(1)), true},
		"uint32 0":    {dbus.MakeVariant(uint32(0)), false},
		"string junk": {dbus.MakeVariant("yes"), false},
	}
	for name, c := range cases {
		if got := truthy(c.v); got != c.want {
			t.Errorf("%s: truthy = %v, want %v", name, got, c.want)
		}
	}
}
