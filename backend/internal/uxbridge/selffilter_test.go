package uxbridge

import "testing"

// Notifications this app posts come back on the session bus attributed to the
// click package. Without the self filter the watch would show them twice.
func TestIsSelfNotification(t *testing.T) {
	self := []string{
		"pulse",
		"pulse.turbosailor",
		// The form the push service actually delivered on device.
		"pulse.turbosailor_pulse",
		"pulse.turbosailor_pulse_0.1.0",
	}
	for _, id := range self {
		if !isSelfNotification(id) {
			t.Errorf("isSelfNotification(%q) = false, want true", id)
		}
	}

	foreign := []string{
		"",
		"Telegram",
		"it.belloworld.mercurygram",
		"ru.yandex.music",
		"ayatana-indicator-sound",
		// A different package that merely starts with the same letters must
		// still get through.
		"pulse.turbosailormeter",
		"pulsemeter.turbosailor",
		// The Waydroid relay is a separate app: its notifications are exactly
		// what this daemon must forward, never filter.
		"waydnotif.turbosailor_waydnotif",
	}
	for _, id := range foreign {
		if isSelfNotification(id) {
			t.Errorf("isSelfNotification(%q) = true, want false", id)
		}
	}
}

// TestRelayNotificationIsUnwrapped pins the card layout the Waydroid relay
// posts: the Android application name is the summary and the message is the
// body. Forwarded verbatim, the watch would name the relay package as the
// application and show its summary as the message title.
func TestRelayNotificationIsUnwrapped(t *testing.T) {
	n, _, overlay, ok := parseNotifyCall([]any{
		"waydnotif.turbosailor_waydnotif", uint32(0), "",
		"Telegram", "Иван: Привет", []string{},
		map[string]any{}, int32(-1),
	})
	if !ok {
		t.Fatal("relay notification must parse")
	}
	if overlay != "" {
		t.Errorf("overlay = %q, want empty", overlay)
	}
	if n.Source != SourceWaydroid {
		t.Errorf("source = %q, want %q", n.Source, SourceWaydroid)
	}
	if n.AppName != "Telegram" {
		t.Errorf("appName = %q, want Telegram", n.AppName)
	}
	if n.Title != "Иван: Привет" {
		t.Errorf("title = %q, want the message", n.Title)
	}
	if n.Body != "" {
		t.Errorf("body = %q, want empty: the message is the title", n.Body)
	}
}

// A native notification must keep the plain freedesktop mapping.
func TestNativeNotificationKeepsShape(t *testing.T) {
	n, _, _, ok := parseNotifyCall([]any{
		"Telegram", uint32(0), "", "Иван", "Привет", []string{},
		map[string]any{}, int32(-1),
	})
	if !ok {
		t.Fatal("native notification must parse")
	}
	if n.Source != SourceFreedesktop {
		t.Errorf("source = %q, want %q", n.Source, SourceFreedesktop)
	}
	if n.AppName != "Telegram" || n.Title != "Иван" || n.Body != "Привет" {
		t.Errorf("unexpected mapping: app=%q title=%q body=%q", n.AppName, n.Title, n.Body)
	}
}
