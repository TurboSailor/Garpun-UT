package uxbridge

import "testing"

// A card the shell closes has to be retracted, otherwise it stays on the watch
// forever: the watch only drops a notification when the phone says so.
func TestClosedNotificationIsRetracted(t *testing.T) {
	b := newTestBridge()
	defer b.Close()

	const (
		caller   = ":1.42"
		serial   = uint32(7)
		serverID = uint32(91)
	)
	id := b.idFor("fd:1")
	b.emit(Notification{ID: id, Source: SourceFreedesktop, AppID: "telegram", Title: "Иван"})
	b.rememberNotifyCall(caller, serial, id)
	b.resolveNotifyCall(caller, serial, serverID)
	drain(b)

	// Expiry is only the popup timing out; the entry lives on in the shade.
	b.onNotificationClosed([]any{serverID, uint32(closeReasonExpired)})
	if got := drain(b); len(got) != 0 {
		t.Fatalf("expiry retracted the notification: %+v", got)
	}

	b.onNotificationClosed([]any{serverID, uint32(closeReasonDismissed)})
	got := drain(b)
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1 retraction", len(got))
	}
	if !got[0].Removed || got[0].ID != id {
		t.Errorf("unexpected retraction: %+v", got[0])
	}
	if got[0].AppID != "telegram" {
		t.Errorf("retraction lost the app id: %+v", got[0])
	}

	// A second close for the same card must not repeat the retraction.
	b.onNotificationClosed([]any{serverID, uint32(closeReasonClosed)})
	if extra := drain(b); len(extra) != 0 {
		t.Errorf("duplicate retraction: %+v", extra)
	}
}

// An unmatched reply must not invent a mapping: every method reply on the bus
// reaches the monitor, not just the ones we care about.
func TestUnknownReplyIsIgnored(t *testing.T) {
	b := newTestBridge()
	defer b.Close()

	b.resolveNotifyCall(":1.9", 3, 55)
	if _, ok := b.bridgeIDForServer(55); ok {
		t.Error("reply to a call we never saw created a mapping")
	}
}

// Dismissing from the watch retracts even when the phone side cannot be
// closed (no bus connection in this test), so the wrist list keeps shrinking.
func TestDismissRetractsWithoutBus(t *testing.T) {
	b := newTestBridge()
	defer b.Close()

	id := b.idFor("fd:1")
	b.emit(Notification{ID: id, Source: SourceFreedesktop, AppID: "telegram", Title: "Иван"})
	drain(b)

	if err := b.DismissNotification(id); err != nil {
		t.Fatalf("DismissNotification: %v", err)
	}
	got := drain(b)
	if len(got) != 1 || !got[0].Removed || got[0].ID != id {
		t.Fatalf("dismiss did not retract: %+v", got)
	}
}
