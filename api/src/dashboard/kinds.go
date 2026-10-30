package dashboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

// Limits are enforced here, by the engine, never trusted to the provider.
const (
	MaxPayloadBytes = 256 * 1024

	MaxListItems   = 10
	MaxChartSeries = 3
	MaxChartPoints = 60
	maxLabelRunes  = 80
	maxTitleRunes  = 120
	maxTextRunes   = 200 // subtitle, caption
	maxShortRunes  = 40  // unit, meta, series name, x label
	maxBadgeRunes  = 30
	maxLinkLength  = 300
)

// StatData is the "stat" kind: one headline number.
type StatData struct {
	Label   string    `json:"label"`
	Value   StatValue `json:"value"`
	Unit    string    `json:"unit,omitempty"`
	Delta   *Delta    `json:"delta,omitempty"`
	Caption string    `json:"caption,omitempty"`
	Link    string    `json:"link,omitempty"`
}

// StatValue is a string or a finite number.
type StatValue struct {
	Text     string
	Number   float64
	IsNumber bool
}

func (v *StatValue) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '"' {
		v.IsNumber = false
		return json.Unmarshal(b, &v.Text)
	}
	if bytes.Equal(b, []byte("null")) {
		return errors.New("must be a string or a number")
	}
	var n float64
	if err := json.Unmarshal(b, &n); err != nil {
		return errors.New("must be a string or a number")
	}
	v.IsNumber, v.Number = true, n
	return nil
}

func (v StatValue) MarshalJSON() ([]byte, error) {
	if v.IsNumber {
		return json.Marshal(v.Number)
	}
	return json.Marshal(v.Text)
}

// Delta is the change shown next to a stat. Good says whether the direction
// is desirable (e.g. fewer errors is "down" and good), so the UI can colour it.
type Delta struct {
	Value     float64 `json:"value"`
	Direction string  `json:"direction"`
	Good      *bool   `json:"good,omitempty"`
}

// ListData is the "list" kind: a short list of rows.
type ListData struct {
	Items []ListItem `json:"items"`
}

type ListItem struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
	Meta     string `json:"meta,omitempty"`
	Badge    *Badge `json:"badge,omitempty"`
	Link     string `json:"link,omitempty"`
}

type Badge struct {
	Text    string `json:"text"`
	Variant string `json:"variant"`
}

// ChartData is the "chart" kind: one or more series over string-labelled x values.
type ChartData struct {
	Type   string        `json:"type"`
	Series []ChartSeries `json:"series"`
}

type ChartSeries struct {
	Name   string       `json:"name"`
	Points []ChartPoint `json:"points"`
}

type ChartPoint struct {
	X string  `json:"x"`
	Y float64 `json:"y"`
}

var (
	deltaDirections = []string{"up", "down", "flat"}
	badgeVariants   = []string{"gray", "green", "amber", "red"}
	chartTypes      = []string{"bar", "line"}
)

// Validate checks raw against kind's schema (a custom kind's only schema is "an
// object, not too deep", see validateCustom) and returns the normalised payload:
// re-marshalled from the typed value, so unknown fields are dropped, strings are
// trimmed, and an unsafe link is removed (a link is optional decoration, so it is
// dropped rather than failing the whole widget). Everything else that is wrong —
// an unknown kind or schema version, a missing required field, a value out of
// range, an over-limit count or length, an oversized payload — is an error, which
// the engine turns into state "error".
func Validate(kind Kind, schemaVersion int, raw json.RawMessage) (json.RawMessage, error) {
	if schemaVersion != SchemaVersion {
		return nil, fmt.Errorf("unsupported schema version %d", schemaVersion)
	}
	if len(raw) > MaxPayloadBytes {
		return nil, fmt.Errorf("payload is %d bytes, over the %d byte limit", len(raw), MaxPayloadBytes)
	}
	switch kind {
	case KindStat:
		return validateInto(raw, validateStat)
	case KindList:
		return validateInto(raw, validateList)
	case KindChart:
		return validateInto(raw, validateChart)
	case KindCustom:
		return validateCustom(raw)
	default:
		return nil, fmt.Errorf("unknown kind %q", kind)
	}
}

// IsEmpty reports whether an already-validated payload has nothing to show, so
// the engine can answer state "empty" instead of "ok".
func IsEmpty(kind Kind, normalised json.RawMessage) bool {
	switch kind {
	case KindCustom:
		var d map[string]json.RawMessage
		return json.Unmarshal(normalised, &d) == nil && len(d) == 0
	case KindList:
		var d ListData
		return json.Unmarshal(normalised, &d) == nil && len(d.Items) == 0
	case KindChart:
		var d ChartData
		if json.Unmarshal(normalised, &d) != nil {
			return false
		}
		for _, s := range d.Series {
			if len(s.Points) > 0 {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// MaxCustomDepth bounds how deeply a custom payload may nest, so a hostile or
// buggy tool cannot hand the browser a structure that is expensive to walk.
const MaxCustomDepth = 8

// validateCustom accepts any JSON object within the depth limit and returns it
// compacted but otherwise untouched: the shape belongs to the tool's renderer.
func validateCustom(raw json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, errors.New("custom data must be a JSON object")
	}
	depth := 0
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if d, ok := tok.(json.Delim); ok {
			if d == '{' || d == '[' {
				depth++
				if depth > MaxCustomDepth {
					return nil, fmt.Errorf("custom data nests deeper than %d levels", MaxCustomDepth)
				}
			} else {
				depth--
			}
		}
	}
	var out bytes.Buffer
	if err := json.Compact(&out, trimmed); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}
	return out.Bytes(), nil
}

func validateInto[T any](raw json.RawMessage, check func(*T) error) (json.RawMessage, error) {
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}
	if err := check(&v); err != nil {
		return nil, err
	}
	out, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}
	return out, nil
}

func validateStat(d *StatData) error {
	var err error
	if d.Label, err = requiredText("label", d.Label, maxLabelRunes); err != nil {
		return err
	}
	if d.Value.IsNumber {
		if math.IsNaN(d.Value.Number) || math.IsInf(d.Value.Number, 0) {
			return errors.New("value must be a finite number")
		}
	} else {
		if d.Value.Text, err = requiredText("value", d.Value.Text, maxLabelRunes); err != nil {
			return err
		}
	}
	if d.Unit, err = optionalText("unit", d.Unit, maxShortRunes); err != nil {
		return err
	}
	if d.Caption, err = optionalText("caption", d.Caption, maxTextRunes); err != nil {
		return err
	}
	if d.Delta != nil {
		if math.IsNaN(d.Delta.Value) || math.IsInf(d.Delta.Value, 0) {
			return errors.New("delta.value must be a finite number")
		}
		if !oneOf(d.Delta.Direction, deltaDirections) {
			return fmt.Errorf("delta.direction must be one of %s", strings.Join(deltaDirections, ", "))
		}
	}
	d.Link = safeLink(d.Link)
	return nil
}

func validateList(d *ListData) error {
	if d.Items == nil {
		return errors.New("items is required")
	}
	if len(d.Items) > MaxListItems {
		return fmt.Errorf("items has %d entries, over the limit of %d", len(d.Items), MaxListItems)
	}
	for i := range d.Items {
		it := &d.Items[i]
		var err error
		if it.Title, err = requiredText(fmt.Sprintf("items[%d].title", i), it.Title, maxTitleRunes); err != nil {
			return err
		}
		if it.Subtitle, err = optionalText(fmt.Sprintf("items[%d].subtitle", i), it.Subtitle, maxTextRunes); err != nil {
			return err
		}
		if it.Meta, err = optionalText(fmt.Sprintf("items[%d].meta", i), it.Meta, maxShortRunes); err != nil {
			return err
		}
		if it.Badge != nil {
			if it.Badge.Text, err = requiredText(fmt.Sprintf("items[%d].badge.text", i), it.Badge.Text, maxBadgeRunes); err != nil {
				return err
			}
			if !oneOf(it.Badge.Variant, badgeVariants) {
				return fmt.Errorf("items[%d].badge.variant must be one of %s", i, strings.Join(badgeVariants, ", "))
			}
		}
		it.Link = safeLink(it.Link)
	}
	return nil
}

func validateChart(d *ChartData) error {
	if !oneOf(d.Type, chartTypes) {
		return fmt.Errorf("type must be one of %s", strings.Join(chartTypes, ", "))
	}
	if d.Series == nil {
		return errors.New("series is required")
	}
	if len(d.Series) > MaxChartSeries {
		return fmt.Errorf("series has %d entries, over the limit of %d", len(d.Series), MaxChartSeries)
	}
	for i := range d.Series {
		s := &d.Series[i]
		var err error
		if s.Name, err = requiredText(fmt.Sprintf("series[%d].name", i), s.Name, maxShortRunes); err != nil {
			return err
		}
		if s.Points == nil {
			return fmt.Errorf("series[%d].points is required", i)
		}
		if len(s.Points) > MaxChartPoints {
			return fmt.Errorf("series[%d].points has %d entries, over the limit of %d", i, len(s.Points), MaxChartPoints)
		}
		for j := range s.Points {
			p := &s.Points[j]
			if p.X, err = requiredText(fmt.Sprintf("series[%d].points[%d].x", i, j), p.X, maxShortRunes); err != nil {
				return err
			}
			if math.IsNaN(p.Y) || math.IsInf(p.Y, 0) {
				return fmt.Errorf("series[%d].points[%d].y must be a finite number", i, j)
			}
		}
	}
	return nil
}

func requiredText(field, s string, maxRunes int) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("%s is required", field)
	}
	return checkLength(field, s, maxRunes)
}

func optionalText(field, s string, maxRunes int) (string, error) {
	return checkLength(field, strings.TrimSpace(s), maxRunes)
}

func checkLength(field, s string, maxRunes int) (string, error) {
	if n := utf8.RuneCountInString(s); n > maxRunes {
		return "", fmt.Errorf("%s is %d characters, over the limit of %d", field, n, maxRunes)
	}
	return s, nil
}

func oneOf(s string, allowed []string) bool {
	for _, a := range allowed {
		if s == a {
			return true
		}
	}
	return false
}

// safeLink keeps only an internal app path: it must start with a single "/",
// and may not be protocol-relative ("//host"), contain a backslash (browsers
// treat "/\host" like "//host"), a control character, or be unreasonably long.
// Anything else — including "https://…" and "javascript:…" — becomes "" (no link).
func safeLink(link string) string {
	link = strings.TrimSpace(link)
	if link == "" || len(link) > maxLinkLength {
		return ""
	}
	if link[0] != '/' || (len(link) > 1 && (link[1] == '/' || link[1] == '\\')) {
		return ""
	}
	for _, r := range link {
		if r == '\\' || r < 0x20 || r == 0x7f {
			return ""
		}
	}
	return link
}
