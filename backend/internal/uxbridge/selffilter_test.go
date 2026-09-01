package uxbridge

import "testing"

// Notifications this app relays through the push service come back on the
// session bus attributed to the click package. Without the self filter the
// watch shows every Android notification twice: once with the real app name
// and once as "cc.zachy.pulse_pulse".
func TestIsSelfNotification(t *testing.T) {
	self := []string{
		"pulse",
		"cc.zachy.pulse",
		// The form the push service actually delivered on device.
		"cc.zachy.pulse_pulse",
		"cc.zachy.pulse_pulse_0.1.0",
		"cc.zachy.pulse_push",
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
		"cc.zachy.pulsemeter",
		"cc.zachy.other_pulse",
	}
	for _, id := range foreign {
		if isSelfNotification(id) {
			t.Errorf("isSelfNotification(%q) = true, want false", id)
		}
	}
}
