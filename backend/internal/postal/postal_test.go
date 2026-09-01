package postal

import (
	"encoding/json"
	"testing"
)

// The push service keys on the click package and rejects a path it does not
// export, so the escaping has to match what it registered.
func TestObjectPath(t *testing.T) {
	cases := []struct {
		appID string
		want  string
	}{
		// Verified against the running service on Ubuntu Touch 24.04.
		{"cc.zachy.pulse_pulse", "/com/lomiri/Postal/cc_2ezachy_2epulse"},
		// The hook suffix is not part of the path.
		{"cc.zachy.pulse", "/com/lomiri/Postal/cc_2ezachy_2epulse"},
		{"com.ubuntu.developer.foo.bar_app", "/com/lomiri/Postal/com_2eubuntu_2edeveloper_2efoo_2ebar"},
		// Already path-safe names pass through untouched.
		{"simple_hook", "/com/lomiri/Postal/simple"},
		// Hyphens are escaped too.
		{"my-app.example_hook", "/com/lomiri/Postal/my_2dapp_2eexample"},
	}
	for _, c := range cases {
		if got := string(objectPath(c.appID)); got != c.want {
			t.Errorf("objectPath(%q) = %q, want %q", c.appID, got, c.want)
		}
	}
}

// The helper passes our payload through untouched, so the JSON here is exactly
// what the shell parses. A wrong shape is logged as "failed to parse
// HelperOutput" and the notification silently disappears.
func TestMessageShape(t *testing.T) {
	var m Message
	m.Notification.Tag = "wd-7"
	m.Notification.Card = &Card{
		Summary: "Telegram",
		Body:    "Ivan: hi",
		Popup:   true,
		Persist: true,
	}

	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	note, ok := back["notification"].(map[string]any)
	if !ok {
		t.Fatalf("missing notification envelope: %s", raw)
	}
	if note["tag"] != "wd-7" {
		t.Errorf("tag = %v", note["tag"])
	}
	card, ok := note["card"].(map[string]any)
	if !ok {
		t.Fatalf("missing card: %s", raw)
	}
	for _, k := range []string{"summary", "body", "popup", "persist"} {
		if _, ok := card[k]; !ok {
			t.Errorf("card is missing %q: %s", k, raw)
		}
	}
	if card["popup"] != true || card["persist"] != true {
		t.Errorf("popup/persist not preserved: %s", raw)
	}
}

// An empty body must not emit a "body" key the shell would render as a blank
// second line.
func TestCardOmitsEmptyBody(t *testing.T) {
	raw, err := json.Marshal(Card{Summary: "App", Popup: true, Persist: true})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var card map[string]any
	if err := json.Unmarshal(raw, &card); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := card["body"]; ok {
		t.Errorf("empty body should be omitted: %s", raw)
	}
}

func TestNewRejectsEmptyAppID(t *testing.T) {
	if _, err := New(""); err == nil {
		t.Fatal("expected an error for an empty application id")
	}
}

// ClearPersistent must skip empty tags rather than asking the service to
// clear "", which it would answer with a count of zero and no error — silently
// hiding a caller bug.
func TestClearPersistentSkipsEmptyTags(t *testing.T) {
	// No bus connection is needed: with only empty tags there is nothing to
	// call, so the loop must return before touching the object.
	c := &Client{appID: "cc.zachy.pulse_pulse"}
	n, err := c.ClearPersistent("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("cleared = %d, want 0", n)
	}
}
