package uxbridge

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"pulse/backend/internal/gfdi"
)

// newTestBridge builds a bridge with no sources attached; tests drive the
// internals directly.
func newTestBridge() *Bridge {
	ctx, cancel := context.WithCancel(context.Background())
	return &Bridge{
		log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		out:     make(chan Notification, 64),
		ids:     map[string]int32{},
		nextID:  1,
		appName: map[string]string{},
		ctx:     ctx,
		cancel:  cancel,
	}
}

func drain(b *Bridge) []Notification {
	var out []Notification
	for {
		select {
		case n := <-b.out:
			out = append(out, n)
		default:
			return out
		}
	}
}

func TestCategoryFor(t *testing.T) {
	cases := []struct {
		appID, appName string
		want           uint8
	}{
		{"com.google.android.apps.messaging", "Messages", gfdi.CategorySMS},
		{"com.android.mms", "Messaging", gfdi.CategorySMS},
		{"com.fsck.k9", "K-9 Mail", gfdi.CategoryEmail},
		{"com.android.dialer", "Phone Calls", gfdi.CategoryIncomingCall},
		{"org.telegram.messenger", "Telegram", gfdi.CategorySocial},
		{"com.whatsapp", "WhatsApp", gfdi.CategorySocial},
		{"org.viber.voip", "Viber", gfdi.CategorySocial},
		{"com.example.signal", "Signal", gfdi.CategorySocial},
		{"com.android.calendar", "Calendar", gfdi.CategorySchedule},
		{"ru.yandex.music", "Music", gfdi.CategoryOther},
		{"", "", gfdi.CategoryOther},
	}
	for _, c := range cases {
		if got := categoryFor(c.appID, c.appName); got != c.want {
			t.Errorf("categoryFor(%q, %q) = %d, want %d", c.appID, c.appName, got, c.want)
		}
	}
}

func TestPrettyAppID(t *testing.T) {
	cases := map[string]string{
		"ru.yandex.music":   "Music",
		"com.android.shell": "Shell",
		"telegram":          "Telegram",
		"":                  "",
		"trailing.":         "Trailing.",
	}
	for in, want := range cases {
		if got := prettyAppID(in); got != want {
			t.Errorf("prettyAppID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRingHistoryAndLookup(t *testing.T) {
	b := newTestBridge()
	defer b.Close()

	for i := range historyLimit + 20 {
		b.emit(Notification{
			ID:     b.idFor("k" + itoa(i)),
			Source: SourceWaydroid,
			AppID:  "com.example.app",
			Title:  "n" + itoa(i),
			TsMs:   int64(i + 1),
		})
	}
	drain(b)

	recent := b.Recent(0)
	if len(recent) != historyLimit {
		t.Fatalf("Recent(0) = %d entries, want %d", len(recent), historyLimit)
	}
	if recent[0].Title != "n"+itoa(historyLimit+19) {
		t.Errorf("newest entry = %q", recent[0].Title)
	}
	if got := b.Recent(5); len(got) != 5 || got[1].Title != "n"+itoa(historyLimit+18) {
		t.Errorf("Recent(5) = %+v", got)
	}

	// The oldest ids have been evicted, the newest are still addressable.
	if c := b.Lookup(1); c != nil {
		t.Errorf("evicted id 1 resolved to %+v", c)
	}
	newest := recent[0]
	c := b.Lookup(newest.ID)
	if c == nil {
		t.Fatalf("newest id %d not found", newest.ID)
	}
	if c.Title != newest.Title || c.AppIdentifier != "com.example.app" || c.Message != "" {
		t.Errorf("content = %+v", c)
	}
	if c.Date.UnixMilli() != newest.TsMs {
		t.Errorf("date = %v, want %d", c.Date, newest.TsMs)
	}
}

func TestLookupSkipsRemoved(t *testing.T) {
	b := newTestBridge()
	defer b.Close()

	id := b.idFor("k")
	b.emit(Notification{ID: id, Source: SourceWaydroid, AppID: "a", Title: "hi"})
	if b.Lookup(id) == nil {
		t.Fatal("live notification not found")
	}
	b.emit(Notification{ID: id, Source: SourceWaydroid, AppID: "a", Title: "hi", Removed: true})
	if c := b.Lookup(id); c != nil {
		t.Errorf("removed notification still resolves: %+v", c)
	}
}

func TestIDForIsStablePerKey(t *testing.T) {
	b := newTestBridge()
	defer b.Close()

	a1, a2 := b.idFor("a"), b.idFor("a")
	bb := b.idFor("b")
	if a1 != a2 {
		t.Errorf("id for the same key changed: %d != %d", a1, a2)
	}
	if a1 == bb {
		t.Errorf("distinct keys share id %d", a1)
	}
	b.forgetKey("a")
	if a3 := b.idFor("a"); a3 == a1 {
		t.Errorf("forgotten key reused id %d", a3)
	}
}

func TestEmitFillsDefaults(t *testing.T) {
	b := newTestBridge()
	defer b.Close()

	b.emit(Notification{ID: 7, Source: SourceWaydroid, AppID: "ru.yandex.music", Title: "x"})
	got := drain(b)
	if len(got) != 1 {
		t.Fatalf("got %d events", len(got))
	}
	if got[0].AppName != "Music" {
		t.Errorf("appName = %q", got[0].AppName)
	}
	if got[0].TsMs == 0 {
		t.Error("timestamp not filled")
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
