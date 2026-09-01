package uxbridge

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

// A notification the phone raised itself carries no pulse hints and must stay
// tagged as a native one.
func TestParseNotifyDefaultsToFreedesktop(t *testing.T) {
	n, ok := parseNotifyCall([]any{
		"Telegram", uint32(0), "", "Ivan", "Hi there",
		[]string{}, map[string]dbus.Variant{}, int32(-1),
	})
	if !ok {
		t.Fatal("expected the call to parse")
	}
	if n.Source != SourceFreedesktop {
		t.Errorf("source = %q, want %q", n.Source, SourceFreedesktop)
	}
	if n.AppName != "Telegram" || n.Title != "Ivan" || n.Body != "Hi there" {
		t.Errorf("unexpected content: %+v", n)
	}
}

// pulse-wdnotify stamps relayed Android notifications, which must survive the
// round trip so the notifyWaydroid setting and the UI counters keep working.
func TestParseNotifyHonoursRelayHints(t *testing.T) {
	n, ok := parseNotifyCall([]any{
		"Telegram", uint32(0), "", "Ivan", "Hi there",
		[]string{},
		map[string]dbus.Variant{
			"desktop-entry": dbus.MakeVariant("org.telegram.messenger"),
			hintSource:      dbus.MakeVariant(SourceWaydroid),
			hintAppName:     dbus.MakeVariant("Telegram"),
		},
		int32(-1),
	})
	if !ok {
		t.Fatal("expected the call to parse")
	}
	if n.Source != SourceWaydroid {
		t.Errorf("source = %q, want %q", n.Source, SourceWaydroid)
	}
	if n.AppID != "org.telegram.messenger" {
		t.Errorf("appID = %q, want the android package", n.AppID)
	}
	if n.AppName != "Telegram" {
		t.Errorf("appName = %q, want %q", n.AppName, "Telegram")
	}
}

// An empty hint must not blank out the app name the caller already supplied.
func TestParseNotifyIgnoresEmptyHints(t *testing.T) {
	n, ok := parseNotifyCall([]any{
		"Signal", uint32(0), "", "Title", "Body",
		[]string{},
		map[string]dbus.Variant{
			hintSource:  dbus.MakeVariant(""),
			hintAppName: dbus.MakeVariant(""),
		},
		int32(-1),
	})
	if !ok {
		t.Fatal("expected the call to parse")
	}
	if n.Source != SourceFreedesktop {
		t.Errorf("source = %q, want the default %q", n.Source, SourceFreedesktop)
	}
	if n.AppName != "Signal" {
		t.Errorf("appName = %q, want %q", n.AppName, "Signal")
	}
}
