package livelog_test

import (
	"encoding/base64"
	"errors"
	"math/rand"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/livelog"
)

func b64(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }

func TestCursorRoundTrip(t *testing.T) {
	for _, c := range []livelog.Cursor{{}, {Offset: 0, Identity: "x"}, {Offset: 1 << 40, Identity: "12:99:3"}, {Offset: 5, Identity: strings.Repeat("i", livelog.MaxIdentityBytes)}} {
		got, present, err := livelog.ParseCursor(c.Encode())
		if err != nil || !present || got != c {
			t.Errorf("%+v -> %+v present=%v err=%v", c, got, present, err)
		}
	}
}

func TestNoCursorIsNotAnError(t *testing.T) {
	if c, present, err := livelog.ParseCursor(""); present || err != nil || c != (livelog.Cursor{}) {
		t.Errorf("empty text: %+v %v %v", c, present, err)
	}
}

func TestEveryTextThatIsNotACursorIsRefused(t *testing.T) {
	good := livelog.Cursor{Offset: 7, Identity: "i"}.Encode()
	for name, text := range map[string]string{
		"not base64":            "!!!not-base64!!!",
		"standard base64 pad":   base64.StdEncoding.EncodeToString([]byte(`{"o":1}`)) + "=",
		"base64 of text":        b64("hello"),
		"json array":            b64(`[1,2]`),
		"json number":           b64(`7`),
		"json null":             b64(`null`),
		"unknown field":         b64(`{"o":1,"x":2}`),
		"wrong type":            b64(`{"o":"1"}`),
		"negative offset":       b64(`{"o":-1}`),
		"float offset":          b64(`{"o":1.5}`),
		"trailing object":       b64(`{"o":1}{"o":2}`),
		"trailing garbage":      b64(`{"o":1}x`),
		"identity too long":     b64(`{"o":1,"i":"` + strings.Repeat("a", livelog.MaxIdentityBytes+1) + `"}`),
		"over the size bound":   strings.Repeat("A", 513),
		"good with a suffix":    good + "A",
		"good with a space":     good + " ",
		"good with a newline":   good + "\n",
		"good in the middle":    "x" + good,
		"padded past the bound": b64(`{"o":1}` + strings.Repeat(" ", 600)),
	} {
		if _, _, err := livelog.ParseCursor(text); !errors.Is(err, livelog.ErrBadCursor) {
			t.Errorf("%s: %v, want ErrBadCursor", name, err)
		}
	}
}

func TestRandomTextNeverPanicsAndNeverAcceptsAnInvalidOffset(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	const alphabet = "ABCabc012-_={}\":,[]o i"
	for i := 0; i < 20000; i++ {
		n := rng.Intn(40)
		b := make([]byte, n)
		for j := range b {
			b[j] = alphabet[rng.Intn(len(alphabet))]
		}
		c, present, err := livelog.ParseCursor(string(b))
		if err == nil && present && (c.Offset < 0 || len(c.Identity) > livelog.MaxIdentityBytes) {
			t.Fatalf("%q accepted as %+v", b, c)
		}
	}
}

// Strict base64: a text that decodes to the same bytes as a real cursor but is not what Encode would write (the unused bits of the
// last character set) is not a cursor either, so one cursor has exactly one text.
func TestOnlyTheCanonicalTextOfACursorIsAccepted(t *testing.T) {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	variants := 0
	for _, c := range []livelog.Cursor{{Offset: 1, Identity: "a"}, {Offset: 12, Identity: "ab"}, {Offset: 123, Identity: "abc"}, {Offset: 7, Identity: "abcd"}} {
		good := c.Encode()
		want, _ := base64.RawURLEncoding.DecodeString(good)
		for i := 0; i < len(alphabet); i++ {
			alt := good[:len(good)-1] + string(alphabet[i])
			if alt == good {
				continue
			}
			got, err := base64.RawURLEncoding.DecodeString(alt) // the lenient decoder reads it as the same bytes
			if err != nil || string(got) != string(want) {
				continue
			}
			variants++
			if _, _, perr := livelog.ParseCursor(alt); !errors.Is(perr, livelog.ErrBadCursor) {
				t.Errorf("%q is a second text for %+v and was accepted", alt, c)
			}
		}
	}
	if variants == 0 {
		t.Fatal("the test found no non-canonical variant to try: the cursors it uses must end in a partial base64 group")
	}
}
