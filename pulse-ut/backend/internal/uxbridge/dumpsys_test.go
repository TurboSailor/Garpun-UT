package uxbridge

import (
	"os"
	"testing"
)

// The fixture is a real `dumpsys notification --noredact` captured from the
// Waydroid container on the target device.
func loadFixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/dumpsys_notification.txt")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return string(b)
}

func TestParseDumpsysNotificationsFixture(t *testing.T) {
	records := parseDumpsysNotifications(loadFixture(t))
	if len(records) != 7 {
		t.Fatalf("got %d records, want 7", len(records))
	}

	byKey := map[string]dumpsysRecord{}
	for _, r := range records {
		if r.Key == "" {
			t.Errorf("record for pkg %q has no key", r.Pkg)
		}
		byKey[r.Key] = r
	}

	music, ok := byKey["0|ru.yandex.music|10501|null|10135"]
	if !ok {
		t.Fatal("music record missing")
	}
	if music.Pkg != "ru.yandex.music" || music.Title != "Attack" || music.Text != "Thirty Seconds to Mars" {
		t.Errorf("music record = %+v", music)
	}
	if music.Flags != 0x8 {
		t.Errorf("music flags = %#x, want 0x8", music.Flags)
	}
	if music.Importance != "LOW" {
		t.Errorf("music importance = %q", music.Importance)
	}

	// A value containing a newline and unbalanced-looking parentheses must
	// survive intact: dumpsys prints embedded newlines verbatim.
	multi, ok := byKey["0|com.android.shell|2020|mtag|2000"]
	if !ok {
		t.Fatal("multiline record missing")
	}
	if want := "line one\nline two (paren) here"; multi.Text != want {
		t.Errorf("multiline text = %q, want %q", multi.Text, want)
	}
	if multi.Title != "Multi" {
		t.Errorf("multiline title = %q", multi.Title)
	}
	if multi.When != 1788283112306 {
		t.Errorf("multiline when = %d", multi.When)
	}

	usb, ok := byKey["-1|android|32|null|1000"]
	if !ok {
		t.Fatal("usb record missing")
	}
	if usb.SubText != "TQ3A.230901.001" {
		t.Errorf("usb subText = %q", usb.SubText)
	}

	// The trailing "mUseAttentionLight=..." section must not leak into the
	// last record.
	last := records[len(records)-1]
	if last.Importance == "" {
		t.Errorf("last record lost its body: %+v", last)
	}
}

func TestDumpsysUserVisibleFilters(t *testing.T) {
	records := parseDumpsysNotifications(loadFixture(t))

	var visible []dumpsysRecord
	for _, r := range records {
		if r.userVisible() {
			visible = append(visible, r)
		}
	}
	if len(visible) != 3 {
		for _, r := range visible {
			t.Logf("visible: pkg=%s title=%q flags=%#x imp=%s", r.Pkg, r.Title, r.Flags, r.Importance)
		}
		t.Fatalf("got %d visible records, want 3", len(visible))
	}

	for _, r := range records {
		switch {
		case r.Pkg == "android" && r.userVisible():
			t.Errorf("system package leaked: %+v", r)
		case r.Flags&flagGroupSummary != 0 && r.userVisible():
			t.Errorf("group summary leaked: %+v", r)
		case r.Importance == "MIN" && r.userVisible():
			t.Errorf("MIN importance leaked: %+v", r)
		}
	}
}

func TestDumpsysBodyPrefersBigText(t *testing.T) {
	r := dumpsysRecord{Text: "short", BigText: "long form", SubText: "sub"}
	if got := r.Body(); got != "long form" {
		t.Errorf("Body() = %q, want big text", got)
	}
	r.BigText = ""
	if got := r.Body(); got != "short" {
		t.Errorf("Body() = %q, want text", got)
	}
	r.Text = ""
	if got := r.Body(); got != "sub" {
		t.Errorf("Body() = %q, want subText", got)
	}
}

func TestParseDumpsysNoSection(t *testing.T) {
	if got := parseDumpsysNotifications("Current Notification Manager state:\n  Zen Mode:\n"); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestDecodeExtraValue(t *testing.T) {
	cases := map[string]string{
		"String (hello)":         "hello",
		"null":                   "",
		"Boolean (true)":         "true",
		"CharSequence (a (b) c)": "a (b) c",
		"ApplicationInfo (ApplicationInfo{1f x})": "ApplicationInfo{1f x}",
		"": "",
	}
	for in, want := range cases {
		if got := decodeExtraValue(in); got != want {
			t.Errorf("decodeExtraValue(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSnapshotAndDiff(t *testing.T) {
	records := parseDumpsysNotifications(loadFixture(t))
	snap := snapshotNotifications(records)
	if len(snap) != 3 {
		t.Fatalf("snapshot has %d entries, want 3", len(snap))
	}

	b := newTestBridge()
	defer b.Close()

	// A baseline diff against itself must be silent.
	b.diffWaydroid(snap, snap)
	if got := drain(b); len(got) != 0 {
		t.Fatalf("self diff emitted %d events", len(got))
	}

	// One appearing, one disappearing, one edited.
	prev := map[string]waydroidState{
		"gone":  {title: "Gone", body: "bye", pkg: "com.example.gone"},
		"edit":  {title: "Edit", body: "old", pkg: "com.example.edit"},
		"stays": {title: "Stays", body: "same", pkg: "com.example.stays"},
	}
	cur := map[string]waydroidState{
		"edit":  {title: "Edit", body: "new", pkg: "com.example.edit"},
		"stays": {title: "Stays", body: "same", pkg: "com.example.stays"},
		"fresh": {title: "Fresh", body: "hi", pkg: "com.example.fresh"},
	}
	b.diffWaydroid(prev, cur)

	got := drain(b)
	if len(got) != 3 {
		t.Fatalf("diff emitted %d events, want 3", len(got))
	}
	seen := map[string]Notification{}
	for _, n := range got {
		seen[n.AppID] = n
	}
	if n := seen["com.example.fresh"]; n.Title != "Fresh" || n.Removed {
		t.Errorf("fresh = %+v", n)
	}
	if n := seen["com.example.edit"]; n.Body != "new" || n.Removed {
		t.Errorf("edit = %+v", n)
	}
	if n := seen["com.example.gone"]; !n.Removed {
		t.Errorf("gone = %+v", n)
	}
	if _, ok := seen["com.example.stays"]; ok {
		t.Error("unchanged notification was re-emitted")
	}

	// The retracted key must not keep its id reserved.
	b.mu.Lock()
	_, reserved := b.ids["wd:gone"]
	b.mu.Unlock()
	if reserved {
		t.Error("id mapping for a removed notification was not released")
	}
}

func TestParseNonLocalizedLabel(t *testing.T) {
	out := "  labelRes=0x7f130272 nonLocalizedLabel=null icon=0x7f100000 banner=0x0\n" +
		"    labelRes=0x0 nonLocalizedLabel=Yandex Music icon=0x1 banner=0x0\n"
	if got := parseNonLocalizedLabel(out); got != "Yandex Music" {
		t.Errorf("got %q", got)
	}
	if got := parseNonLocalizedLabel("labelRes=0x1 nonLocalizedLabel=null icon=0x0 banner=0x0"); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
