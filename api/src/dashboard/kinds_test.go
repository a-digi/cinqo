package dashboard

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func validate(t *testing.T, kind Kind, payload string) (string, error) {
	t.Helper()
	out, err := Validate(kind, SchemaVersion, json.RawMessage(payload))
	return string(out), err
}

func TestParseKind(t *testing.T) {
	for _, k := range Kinds() {
		if got, ok := ParseKind(string(k)); !ok || got != k {
			t.Errorf("ParseKind(%q) = %q, %v", k, got, ok)
		}
	}
	if _, ok := ParseKind("table"); ok {
		t.Error("unknown kind accepted")
	}
}

func TestValidateRejectsBadEnvelope(t *testing.T) {
	if _, err := Validate(KindStat, 2, json.RawMessage(`{"label":"a","value":1}`)); err == nil {
		t.Error("unknown schema version accepted")
	}
	if _, err := Validate(Kind("table"), SchemaVersion, json.RawMessage(`{}`)); err == nil {
		t.Error("unknown kind accepted")
	}
	big := `{"label":"a","value":"` + strings.Repeat("x", MaxPayloadBytes) + `"}`
	if _, err := Validate(KindStat, SchemaVersion, json.RawMessage(big)); err == nil {
		t.Error("oversized payload accepted")
	}
	for _, bad := range []string{``, `[]`, `"x"`, `{`} {
		if _, err := Validate(KindStat, SchemaVersion, json.RawMessage(bad)); err == nil {
			t.Errorf("payload %q accepted", bad)
		}
	}
}

func TestValidateStat(t *testing.T) {
	good, err := validate(t, KindStat, `{"label":" Jobs ","value":42,"unit":"open","delta":{"value":-3,"direction":"down","good":true},"caption":"this week","link":"/tools/career/jobs","extra":"dropped"}`)
	if err != nil {
		t.Fatalf("valid stat rejected: %v", err)
	}
	if strings.Contains(good, "extra") || !strings.Contains(good, `"label":"Jobs"`) {
		t.Errorf("not normalised: %s", good)
	}
	if out, err := validate(t, KindStat, `{"label":"Status","value":"Healthy"}`); err != nil || !strings.Contains(out, `"value":"Healthy"`) {
		t.Errorf("string value: %v %s", err, out)
	}
	if out, _ := validate(t, KindStat, `{"label":"Zero","value":0}`); !strings.Contains(out, `"value":0`) {
		t.Errorf("zero value must survive: %s", out)
	}

	for name, payload := range map[string]string{
		"missing label": `{"value":1}`,
		"blank label":   `{"label":"  ","value":1}`,
		"missing value": `{"label":"a"}`,
		"null value":    `{"label":"a","value":null}`,
		"bool value":    `{"label":"a","value":true}`,
		"blank value":   `{"label":"a","value":" "}`,
		"long label":    fmt.Sprintf(`{"label":%q,"value":1}`, strings.Repeat("l", 81)),
		"long unit":     fmt.Sprintf(`{"label":"a","value":1,"unit":%q}`, strings.Repeat("u", 41)),
		"long caption":  fmt.Sprintf(`{"label":"a","value":1,"caption":%q}`, strings.Repeat("c", 201)),
		"bad direction": `{"label":"a","value":1,"delta":{"value":1,"direction":"sideways"}}`,
		"missing dir":   `{"label":"a","value":1,"delta":{"value":1}}`,
		"object value":  `{"label":"a","value":{"n":1}}`,
	} {
		if _, err := validate(t, KindStat, payload); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// Multi-byte text is counted in characters, not bytes.
	if _, err := validate(t, KindStat, fmt.Sprintf(`{"label":%q,"value":1}`, strings.Repeat("é", 80))); err != nil {
		t.Errorf("80 multi-byte characters rejected: %v", err)
	}
}

func TestValidateList(t *testing.T) {
	out, err := validate(t, KindList, `{"items":[{"title":"Backend Engineer","subtitle":"Acme","meta":"2d","badge":{"text":"New","variant":"green"},"link":"/tools/career/jobs"},{"title":"Plain"}]}`)
	if err != nil {
		t.Fatalf("valid list rejected: %v", err)
	}
	if IsEmpty(KindList, json.RawMessage(out)) {
		t.Error("non-empty list reported empty")
	}
	if out, err := validate(t, KindList, `{"items":[]}`); err != nil || !IsEmpty(KindList, json.RawMessage(out)) {
		t.Errorf("empty list: %v empty=%v", err, IsEmpty(KindList, json.RawMessage(out)))
	}

	items := func(n int) string {
		parts := make([]string, n)
		for i := range parts {
			parts[i] = `{"title":"t"}`
		}
		return `{"items":[` + strings.Join(parts, ",") + `]}`
	}
	if _, err := validate(t, KindList, items(MaxListItems)); err != nil {
		t.Errorf("max items rejected: %v", err)
	}
	for name, payload := range map[string]string{
		"too many":     items(MaxListItems + 1),
		"no items":     `{}`,
		"null items":   `{"items":null}`,
		"blank title":  `{"items":[{"title":" "}]}`,
		"long title":   fmt.Sprintf(`{"items":[{"title":%q}]}`, strings.Repeat("t", 121)),
		"long sub":     fmt.Sprintf(`{"items":[{"title":"a","subtitle":%q}]}`, strings.Repeat("s", 201)),
		"long meta":    fmt.Sprintf(`{"items":[{"title":"a","meta":%q}]}`, strings.Repeat("m", 41)),
		"bad variant":  `{"items":[{"title":"a","badge":{"text":"x","variant":"purple"}}]}`,
		"blank badge":  `{"items":[{"title":"a","badge":{"text":"","variant":"red"}}]}`,
		"long badge":   fmt.Sprintf(`{"items":[{"title":"a","badge":{"text":%q,"variant":"red"}}]}`, strings.Repeat("b", 31)),
		"items string": `{"items":"nope"}`,
	} {
		if _, err := validate(t, KindList, payload); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestValidateChart(t *testing.T) {
	out, err := validate(t, KindChart, `{"type":"line","series":[{"name":"Jobs","points":[{"x":"Mon","y":3},{"x":"Tue","y":0}]}]}`)
	if err != nil {
		t.Fatalf("valid chart rejected: %v", err)
	}
	if IsEmpty(KindChart, json.RawMessage(out)) {
		t.Error("chart with points reported empty")
	}
	for name, payload := range map[string]string{
		"no series":   `{"type":"bar","series":[]}`,
		"series only": `{"type":"bar","series":[{"name":"a","points":[]}]}`,
	} {
		out, err := validate(t, KindChart, payload)
		if err != nil || !IsEmpty(KindChart, json.RawMessage(out)) {
			t.Errorf("%s: want valid+empty, got err=%v", name, err)
		}
	}

	points := func(n int) string {
		parts := make([]string, n)
		for i := range parts {
			parts[i] = fmt.Sprintf(`{"x":"%d","y":1}`, i)
		}
		return strings.Join(parts, ",")
	}
	series := func(n int) string {
		parts := make([]string, n)
		for i := range parts {
			parts[i] = fmt.Sprintf(`{"name":"s%d","points":[]}`, i)
		}
		return strings.Join(parts, ",")
	}
	if _, err := validate(t, KindChart, fmt.Sprintf(`{"type":"bar","series":[{"name":"a","points":[%s]}]}`, points(MaxChartPoints))); err != nil {
		t.Errorf("max points rejected: %v", err)
	}
	if _, err := validate(t, KindChart, fmt.Sprintf(`{"type":"bar","series":[%s]}`, series(MaxChartSeries))); err != nil {
		t.Errorf("max series rejected: %v", err)
	}
	for name, payload := range map[string]string{
		"bad type":        `{"type":"pie","series":[]}`,
		"missing type":    `{"series":[]}`,
		"null series":     `{"type":"bar","series":null}`,
		"too many series": fmt.Sprintf(`{"type":"bar","series":[%s]}`, series(MaxChartSeries+1)),
		"too many points": fmt.Sprintf(`{"type":"bar","series":[{"name":"a","points":[%s]}]}`, points(MaxChartPoints+1)),
		"no name":         `{"type":"bar","series":[{"name":"","points":[]}]}`,
		"null points":     `{"type":"bar","series":[{"name":"a","points":null}]}`,
		"no x":            `{"type":"bar","series":[{"name":"a","points":[{"x":"","y":1}]}]}`,
		"string y":        `{"type":"bar","series":[{"name":"a","points":[{"x":"a","y":"1"}]}]}`,
		"long x":          fmt.Sprintf(`{"type":"bar","series":[{"name":"a","points":[{"x":%q,"y":1}]}]}`, strings.Repeat("x", 41)),
	} {
		if _, err := validate(t, KindChart, payload); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestLinksAreInternalOnly(t *testing.T) {
	for link, want := range map[string]string{
		"/tools/career/jobs":                     "/tools/career/jobs",
		"/":                                      "/",
		"  /admin/tools  ":                       "/admin/tools",
		"/jobs?status=open#top":                  "/jobs?status=open#top",
		"":                                       "",
		"https://evil.example":                   "",
		"//evil.example":                         "",
		"/\\evil.example":                        "",
		"javascript:alert(1)":                    "",
		"data:text/html,x":                       "",
		"relative/path":                          "",
		"/ok\nSet-Cookie: x":                     "",
		"/ok\tx":                                 "",
		"/" + strings.Repeat("a", maxLinkLength): "",
	} {
		if got := safeLink(link); got != want {
			t.Errorf("safeLink(%q) = %q, want %q", link, got, want)
		}
	}

	// An unsafe link is dropped, not fatal, on every kind that has one.
	out, err := validate(t, KindStat, `{"label":"a","value":1,"link":"https://evil.example"}`)
	if err != nil || strings.Contains(out, "link") {
		t.Errorf("stat: err=%v out=%s", err, out)
	}
	out, err = validate(t, KindList, `{"items":[{"title":"a","link":"javascript:alert(1)"}]}`)
	if err != nil || strings.Contains(out, "link") {
		t.Errorf("list: err=%v out=%s", err, out)
	}
}

func TestNonObjectPayloadForEveryKind(t *testing.T) {
	for _, k := range Kinds() {
		for _, bad := range []string{`null`, `[]`, `1`} {
			if _, err := Validate(k, SchemaVersion, json.RawMessage(bad)); err == nil {
				t.Errorf("%s accepted %s", k, bad)
			}
		}
	}
}

func TestValidateCustom(t *testing.T) {
	out, err := validate(t, KindCustom, ` { "rows": [ {"a": 1, "b": [true, null, "x"]} ], "title": "Hi", "n": 1.50 } `)
	if err != nil {
		t.Fatalf("valid custom rejected: %v", err)
	}
	if out != `{"rows":[{"a":1,"b":[true,null,"x"]}],"title":"Hi","n":1.50}` {
		t.Errorf("custom data must be kept as sent, only compacted: %s", out)
	}
	for name, payload := range map[string]string{
		"array":  `[1]`,
		"string": `"x"`,
		"number": `1`,
		"null":   `null`,
		"empty":  ``,
		"broken": `{"a":`,
	} {
		if _, err := validate(t, KindCustom, payload); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}

	nest := func(levels int) string {
		return strings.Repeat(`{"a":`, levels) + `1` + strings.Repeat(`}`, levels)
	}
	if _, err := validate(t, KindCustom, nest(MaxCustomDepth)); err != nil {
		t.Errorf("depth %d rejected: %v", MaxCustomDepth, err)
	}
	if _, err := validate(t, KindCustom, nest(MaxCustomDepth+1)); err == nil {
		t.Errorf("depth %d accepted", MaxCustomDepth+1)
	}
	arrays := `{"a":` + strings.Repeat(`[`, MaxCustomDepth) + strings.Repeat(`]`, MaxCustomDepth) + `}`
	if _, err := validate(t, KindCustom, arrays); err == nil {
		t.Error("arrays count toward depth")
	}

	if empty, err := validate(t, KindCustom, `{}`); err != nil || !IsEmpty(KindCustom, json.RawMessage(empty)) {
		t.Errorf("an empty object is valid and empty: %v", err)
	}
	if full, _ := validate(t, KindCustom, `{"a":1}`); IsEmpty(KindCustom, json.RawMessage(full)) {
		t.Error("a non-empty object reported empty")
	}
	if _, ok := ParseKind("custom"); !ok {
		t.Error("custom is not a known kind")
	}
}
