package uxbridge

import (
	"strconv"
	"strings"
)

// Parser for `dumpsys notification --noredact`. Only the "Notification List:"
// section is interesting: it holds the currently posted records. Everything
// below it (snoozed, ranking config, usage stats) is deliberately ignored.

// flagGroupSummary is Notification.FLAG_GROUP_SUMMARY. Those records carry no
// text of their own, they only exist to collapse a group in the shade.
const flagGroupSummary = 0x200

// dumpsysRecord is one NotificationRecord as printed by the notification
// manager.
type dumpsysRecord struct {
	Key        string
	Pkg        string
	Tag        string
	ID         int
	Flags      int
	When       int64
	Importance string
	Title      string
	Text       string
	BigText    string
	SubText    string
	InfoText   string
	GroupKey   string
}

// Body picks the most informative text the notification carries.
func (r dumpsysRecord) Body() string {
	for _, s := range []string{r.BigText, r.Text, r.SubText, r.InfoText} {
		if s != "" {
			return s
		}
	}
	return ""
}

// userVisible filters out the records a watch must never see: OS plumbing,
// group summaries and channels the user already silenced.
func (r dumpsysRecord) userVisible() bool {
	switch r.Pkg {
	case "", "android", "com.android.systemui", "com.android.providers.downloads":
		return false
	}
	if r.Flags&flagGroupSummary != 0 {
		return false
	}
	switch r.Importance {
	case "MIN", "NONE", "UNSPECIFIED":
		return false
	}
	return r.Title != "" || r.Body() != ""
}

func parseDumpsysNotifications(out string) []dumpsysRecord {
	lines := strings.Split(out, "\n")

	start := -1
	for i, ln := range lines {
		if strings.TrimSpace(strings.TrimRight(ln, "\r")) == "Notification List:" {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return nil
	}

	var (
		records []dumpsysRecord
		block   []string
	)
	flush := func() {
		if len(block) > 0 {
			records = append(records, parseDumpsysRecord(block))
			block = nil
		}
	}
	for _, raw := range lines[start:] {
		ln := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(ln)
		if trimmed == "" {
			if len(block) > 0 {
				block = append(block, ln)
			}
			continue
		}
		indent := indentOf(ln)
		if indent == 4 && strings.HasPrefix(trimmed, "NotificationRecord(") {
			flush()
			block = append(block, ln)
			continue
		}
		// Column 0 means a multi-line extras value wrapping past the margin;
		// any other shallow line is the next top-level dumpsys section.
		if indent >= 1 && indent < 4 {
			break
		}
		if len(block) > 0 {
			block = append(block, ln)
		}
	}
	flush()
	return records
}

func indentOf(s string) int {
	for i, c := range s {
		if c != ' ' {
			return i
		}
	}
	return len(s)
}

func parseDumpsysRecord(block []string) dumpsysRecord {
	r := dumpsysRecord{ID: -1}

	// The header carries pkg/id/tag/importance; everything else is read from
	// the indented body, where each field sits alone on its line.
	if len(block) > 0 {
		h := block[0]
		r.Pkg = headerField(h, "pkg=")
		r.Tag = nullable(headerField(h, "tag="))
		if v, err := strconv.Atoi(headerField(h, "id=")); err == nil {
			r.ID = v
		}
	}

	for i := 1; i < len(block); i++ {
		ln := block[i]
		trimmed := strings.TrimSpace(ln)
		switch {
		case strings.HasPrefix(trimmed, "key="):
			if r.Key == "" {
				r.Key = strings.TrimPrefix(trimmed, "key=")
			}
		case strings.HasPrefix(trimmed, "groupKey="):
			r.GroupKey = strings.TrimPrefix(trimmed, "groupKey=")
		case strings.HasPrefix(trimmed, "flags=0x"):
			if v, err := strconv.ParseInt(trimmed[len("flags=0x"):], 16, 32); err == nil {
				r.Flags = int(v)
			}
		case strings.HasPrefix(trimmed, "when="):
			if v, err := strconv.ParseInt(strings.TrimPrefix(trimmed, "when="), 10, 64); err == nil {
				r.When = v
			}
		case strings.HasPrefix(trimmed, "mImportance="):
			r.Importance = strings.TrimPrefix(trimmed, "mImportance=")
		case trimmed == "extras={":
			var extras map[string]string
			extras, i = parseExtras(block, i)
			r.Title = extras["android.title"]
			r.Text = extras["android.text"]
			r.BigText = extras["android.bigText"]
			r.SubText = extras["android.subText"]
			r.InfoText = extras["android.infoText"]
		}
	}
	return r
}

// parseExtras reads the extras bundle starting at block[open] ("extras={") and
// returns the decoded values plus the index of the closing brace. Values may
// span several lines because Android prints embedded newlines verbatim.
func parseExtras(block []string, open int) (map[string]string, int) {
	extras := map[string]string{}
	baseIndent := indentOf(block[open])

	var key string
	var value []string
	commit := func() {
		if key != "" {
			extras[key] = decodeExtraValue(strings.Join(value, "\n"))
		}
		key, value = "", nil
	}

	i := open + 1
	for ; i < len(block); i++ {
		ln := block[i]
		trimmed := strings.TrimSpace(ln)
		if trimmed == "}" && indentOf(ln) == baseIndent {
			break
		}
		if k, v, ok := extraEntry(ln, baseIndent); ok {
			commit()
			key, value = k, []string{v}
			continue
		}
		if key != "" {
			value = append(value, ln)
		}
	}
	commit()
	return extras, i
}

// extraEntry recognises "        android.title=String (…)" at exactly one
// indentation step below the bundle. Continuation lines of a multi-line value
// never match because they are not indented that way.
func extraEntry(ln string, baseIndent int) (key, value string, ok bool) {
	if indentOf(ln) != baseIndent+4 {
		return "", "", false
	}
	trimmed := strings.TrimSpace(ln)
	eq := strings.IndexByte(trimmed, '=')
	if eq <= 0 {
		return "", "", false
	}
	key = trimmed[:eq]
	for _, c := range key {
		if c != '.' && c != '_' && !isAlnum(c) {
			return "", "", false
		}
	}
	return key, trimmed[eq+1:], true
}

func isAlnum(c rune) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// decodeExtraValue turns `String (hello)` into `hello` and `null` into "".
// Non-textual types (Bundle, Icon, ApplicationInfo, …) go through the same
// unwrap; callers only ever look up text keys.
func decodeExtraValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return ""
	}
	open := strings.IndexByte(raw, '(')
	if open < 0 || !strings.HasSuffix(raw, ")") {
		return raw
	}
	return strings.TrimSpace(raw[open+1 : len(raw)-1])
}

// headerField pulls "name=value" out of the NotificationRecord header line,
// where values are whitespace delimited.
func headerField(line, prefix string) string {
	idx := strings.Index(line, prefix)
	if idx < 0 {
		return ""
	}
	rest := line[idx+len(prefix):]
	if end := strings.IndexByte(rest, ' '); end >= 0 {
		rest = rest[:end]
	}
	return rest
}

func nullable(s string) string {
	if s == "null" {
		return ""
	}
	return s
}
