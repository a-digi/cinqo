package jsonview

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

func format(t *testing.T, in string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	_, err := Format(context.Background(), &out, strings.NewReader(in), Limits{})
	return out.String(), err
}

// oracle is what encoding/json makes of valid JSON: the same indentation and no trailing newline.
func oracle(t *testing.T, in []byte) string {
	t.Helper()
	var compact bytes.Buffer
	if err := json.Compact(&compact, in); err != nil {
		t.Fatalf("oracle: %v", err)
	}
	var out bytes.Buffer
	if err := json.Indent(&out, compact.Bytes(), "", "  "); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

// ---- generated documents ----

func randString(r *rand.Rand) string {
	pieces := []string{"a", "b c", "é", "日本", `\"`, `\\`, `\/`, `\n`, `\t`, `>`, `é`, "{", "}", "[", "]", ",", ":", " ", "<b>x</b>", "0"}
	n := r.Intn(8)
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(pieces[r.Intn(len(pieces))])
	}
	return b.String()
}

func randNumber(r *rand.Rand) string {
	switch r.Intn(7) {
	case 0:
		return "0"
	case 1:
		return "-0"
	case 2:
		return fmt.Sprintf("%d", r.Intn(100000)-50000)
	case 3:
		return fmt.Sprintf("%d.%d", r.Intn(1000), r.Intn(1000))
	case 4:
		return fmt.Sprintf("%de%d", r.Intn(100), r.Intn(20)-10)
	case 5:
		return fmt.Sprintf("-%d.%dE+%d", r.Intn(100), r.Intn(100), r.Intn(20))
	}
	return fmt.Sprintf("%d", r.Intn(10))
}

func ws(r *rand.Rand) string {
	return []string{"", "", " ", "\n", "\t", "\r\n", "  \n  "}[r.Intn(7)]
}

func randValue(r *rand.Rand, depth int, spaced bool) string {
	sp := func() string {
		if spaced {
			return ws(r)
		}
		return ""
	}
	k := r.Intn(8)
	if depth <= 0 && k >= 5 {
		k = r.Intn(5)
	}
	switch k {
	case 0:
		return `"` + randString(r) + `"`
	case 1:
		return randNumber(r)
	case 2:
		return "true"
	case 3:
		return "false"
	case 4:
		return "null"
	case 5, 6:
		n := r.Intn(5)
		parts := make([]string, n)
		for i := range parts {
			parts[i] = sp() + randValue(r, depth-1, spaced) + sp()
		}
		return "[" + sp() + strings.Join(parts, ",") + sp() + "]"
	}
	n := r.Intn(5)
	parts := make([]string, n)
	for i := range parts {
		parts[i] = sp() + `"` + randString(r) + `"` + sp() + ":" + sp() + randValue(r, depth-1, spaced) + sp()
	}
	return "{" + sp() + strings.Join(parts, ",") + sp() + "}"
}

func TestFormatEqualsEncodingJSONIndentOnGeneratedDocuments(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 3000; i++ {
		in := sp(r) + randValue(r, 1+r.Intn(5), i%2 == 0) + sp(r)
		got, err := format(t, in)
		if err != nil {
			t.Fatalf("doc %d %q: %v", i, in, err)
		}
		if want := oracle(t, []byte(in)); got != want {
			t.Fatalf("doc %d\ninput: %q\n got: %q\nwant: %q", i, in, got, want)
		}
	}
}

func sp(r *rand.Rand) string { return ws(r) }

func TestAnyCutOfTheInputIntoPiecesGivesTheSameOutput(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	for i := 0; i < 300; i++ {
		in := randValue(r, 4, true)
		want := oracle(t, []byte(in))
		var out bytes.Buffer
		if _, err := Format(context.Background(), &out, iotest.OneByteReader(strings.NewReader(in)), Limits{}); err != nil || out.String() != want {
			t.Fatalf("one byte at a time, doc %q: %v", in, err)
		}
		out.Reset()
		if _, err := Format(context.Background(), &out, iotest.HalfReader(strings.NewReader(in)), Limits{}); err != nil || out.String() != want {
			t.Fatalf("half reads, doc %q: %v", in, err)
		}
	}
}

func TestTheOutputIsFormattedAlreadyAndFormattingItAgainChangesNothing(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	for i := 0; i < 300; i++ {
		once, err := format(t, randValue(r, 4, true))
		if err != nil {
			t.Fatal(err)
		}
		twice, err := format(t, once)
		if err != nil || twice != once {
			t.Fatalf("not idempotent: %q then %q (%v)", once, twice, err)
		}
	}
}

func TestTopLevelScalarsAndEmptyContainers(t *testing.T) {
	for in, want := range map[string]string{
		`"x"`: `"x"`, `12`: `12`, ` true `: `true`, `null`: `null`, `-0.5e-3`: `-0.5e-3`,
		`{}`: `{}`, `[]`: `[]`, `{ }`: `{}`, `[ ]`: `[]`, `[{},[]]`: "[\n  {},\n  []\n]",
		`{"a":{"b":[1,2]}}`: "{\n  \"a\": {\n    \"b\": [\n      1,\n      2\n    ]\n  }\n}",
	} {
		got, err := format(t, in)
		if err != nil || got != want {
			t.Errorf("%q: got %q (%v), want %q", in, got, err, want)
		}
	}
}

func TestAStringIsCopiedByteForByte(t *testing.T) {
	in := `{"html":"<p class=\"x\">café > \\ \/ \n</p>","raw":"café 日本 </b>"}`
	got, err := format(t, in)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{`"<p class=\"x\">café > \\ \/ \n</p>"`, `"café 日本 </b>"`} {
		if !strings.Contains(got, s) {
			t.Errorf("%s was changed in %q", s, got)
		}
	}
}

// ---- real files ----

func TestRealCrawlLogsMatchTheOracle(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "..", "app", "data", "logs", "tools", "browser", "crawl_logs")
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	if len(files) == 0 {
		t.Skip("no real crawl logs in this checkout")
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil || !json.Valid(raw) {
			continue
		}
		var out bytes.Buffer
		if _, err := Format(context.Background(), &out, bytes.NewReader(raw), Limits{}); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if out.String() != oracle(t, raw) {
			t.Fatalf("%s: differs from encoding/json", f)
		}
	}
	t.Logf("%d real crawl logs agree with encoding/json", len(files))
}

// ---- invalid input ----

func TestInvalidInputGivesTheOffsetAndWhatWasExpected(t *testing.T) {
	cases := []struct {
		in       string
		offset   int64
		expected string
	}{
		{``, 0, "a value"},
		{`   `, 3, "a value"},
		{`{`, 1, "a key"},
		{`[`, 1, "a value or ]"},
		{`]`, 0, "a value"},
		{`}`, 0, "a value"},
		{`{"a"}`, 4, "a colon"},
		{`{"a":}`, 5, "a value"},
		{`{"a":1,}`, 7, "a key"},
		{`{a:1}`, 1, "a key"},
		{`{"a" 1}`, 5, "a colon"},
		{`[1,]`, 3, "a value"},
		{`[1 2]`, 3, "a comma or a closing bracket"},
		{`[1}`, 2, "a comma or a closing bracket"},
		{`{"a":1]`, 6, "a comma or a closing bracket"},
		{`"abc`, 4, "a closing quote"},
		{`"a\q"`, 3, "a valid escape"},
		{`"\u12G4"`, 5, "four hex digits"},
		{`"\u12`, 5, "four hex digits"},
		{`"a` + "\x01" + `b"`, 2, "no raw control character"},
		{`"a\`, 3, "an escape"},
		{`tru`, 3, "true"},
		{`trux`, 3, "true"},
		{`nul`, 3, "null"},
		{`falsy`, 4, "false"},
		{`01`, 1, "the end of the input"},
		{`-`, 1, "a digit"},
		{`-x`, 1, "a digit"},
		{`1.`, 2, "a digit"},
		{`1.x`, 2, "a digit"},
		{`1e`, 2, "a digit"},
		{`1e+`, 3, "a digit"},
		{`{} x`, 3, "the end of the input"},
		{`1 2`, 2, "the end of the input"},
		{`[1]]`, 3, "the end of the input"},
	}
	for _, c := range cases {
		if json.Valid([]byte(c.in)) {
			t.Fatalf("%q is valid: bad test case", c.in)
		}
		_, err := format(t, c.in)
		var se *SyntaxError
		if !errors.As(err, &se) {
			t.Errorf("%q: want a SyntaxError, got %v", c.in, err)
			continue
		}
		if se.Offset != c.offset || !strings.Contains(se.Expected, c.expected) {
			t.Errorf("%q: got byte %d expecting %q, want byte %d expecting %q", c.in, se.Offset, se.Expected, c.offset, c.expected)
		}
	}
}

func TestOnAMutatedDocumentWeAgreeWithEncodingJSONAboutValidity(t *testing.T) {
	r := rand.New(rand.NewSource(4))
	agreeInvalid := 0
	for i := 0; i < 6000; i++ {
		doc := []byte(randValue(r, 4, i%2 == 0))
		if len(doc) == 0 {
			continue
		}
		switch r.Intn(3) {
		case 0: // delete a byte
			p := r.Intn(len(doc))
			doc = append(doc[:p:p], doc[p+1:]...)
		case 1: // replace a byte
			doc[r.Intn(len(doc))] = "{}[],:\"\\0-e. x\n\x00"[r.Intn(16)]
		default: // insert a byte
			p := r.Intn(len(doc) + 1)
			doc = append(doc[:p:p], append([]byte{"{}[],:\"\\0-e. x"[r.Intn(14)]}, doc[p:]...)...)
		}
		var out bytes.Buffer
		_, err := Format(context.Background(), &out, bytes.NewReader(doc), Limits{})
		var se *SyntaxError
		isSyntax := errors.As(err, &se)
		if json.Valid(doc) {
			if err != nil || out.String() != oracle(t, doc) {
				t.Fatalf("valid doc %q: %v", doc, err)
			}
			continue
		}
		if !isSyntax {
			t.Fatalf("invalid doc %q: want a SyntaxError, got %v", doc, err)
		}
		if se.Offset < 0 || se.Offset > int64(len(doc)) {
			t.Fatalf("offset %d outside %q", se.Offset, doc)
		}
		agreeInvalid++
	}
	if agreeInvalid < 1000 {
		t.Fatalf("only %d invalid mutations: the generator is too tame", agreeInvalid)
	}
}

func TestAnErrorNeverEchoesTheInput(t *testing.T) {
	secret := "TOPSECRETVALUE"
	for _, in := range []string{`{"` + secret + `":}`, `["` + secret + `" x]`, `"` + secret + `\q"`, secret} {
		_, err := format(t, in)
		if err == nil || strings.Contains(err.Error(), secret) {
			t.Errorf("%q: %v", in, err)
		}
		var se *SyntaxError
		if errors.As(err, &se) && strings.Contains(se.Expected, secret) {
			t.Errorf("Expected echoes the input: %q", se.Expected)
		}
	}
}

// ---- limits ----

func TestNestingDeeperThanTheLimitIsRefused(t *testing.T) {
	at := strings.Repeat("[", DefaultMaxDepth) + strings.Repeat("]", DefaultMaxDepth)
	if _, err := format(t, at); err != nil {
		t.Fatalf("exactly the limit: %v", err)
	}
	over := strings.Repeat("[", DefaultMaxDepth+1) + strings.Repeat("]", DefaultMaxDepth+1)
	if _, err := format(t, over); !errors.Is(err, ErrTooDeep) {
		t.Fatalf("one over: %v", err)
	}
	// a hostile document that never closes is refused at the limit, not buffered
	if _, err := format(t, strings.Repeat("{\"a\":", 100000)); !errors.Is(err, ErrTooDeep) {
		t.Fatalf("endless nesting: %v", err)
	}
	var out bytes.Buffer
	if _, err := Format(context.Background(), &out, strings.NewReader(`[[[1]]]`), Limits{MaxDepth: 2}); !errors.Is(err, ErrTooDeep) {
		t.Fatalf("custom depth: %v", err)
	}
}

func TestInputLongerThanTheLimitIsRefused(t *testing.T) {
	doc := `["` + strings.Repeat("x", 1000) + `"]`
	var out bytes.Buffer
	if _, err := Format(context.Background(), &out, strings.NewReader(doc), Limits{MaxBytes: int64(len(doc))}); err != nil {
		t.Fatalf("exactly the limit: %v", err)
	}
	out.Reset()
	if _, err := Format(context.Background(), &out, strings.NewReader(doc), Limits{MaxBytes: int64(len(doc)) - 1}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("one over: %v", err)
	}
}

func TestACancelledContextStopsAtOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()
	done := make(chan error, 1)
	go func() {
		_, err := Format(ctx, io.Discard, pr, Limits{})
		done <- err
	}()
	go func() {
		pw.Write([]byte(`["a",`))
		time.Sleep(50 * time.Millisecond)
		cancel()
		pw.Write([]byte(`"b"]`)) // a reader that goes on after the cancel
		pw.Close()
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("did not stop")
	}
}

func TestAContextAlreadyCancelledReadsNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Format(ctx, io.Discard, strings.NewReader(`[1]`), Limits{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestReadAndWriteErrorsPass(t *testing.T) {
	boom := errors.New("boom")
	if _, err := Format(context.Background(), io.Discard, io.MultiReader(strings.NewReader(`["a",`), iotest.ErrReader(boom)), Limits{}); !errors.Is(err, boom) {
		t.Fatalf("read error: %v", err)
	}
	big := `[` + strings.Repeat(`"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",`, 5000) + `"y"]`
	if _, err := Format(context.Background(), failWriter{}, strings.NewReader(big), Limits{}); !errors.Is(err, errWrite) {
		t.Fatalf("write error: %v", err)
	}
}

var errWrite = errors.New("disk full")

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errWrite }

// ---- memory ----

// docReader makes a large valid document on the fly, so the test never holds it.
type docReader struct {
	left   int64
	stage  int
	buf    []byte
	record []byte // the record and its comma, made once
}

func (d *docReader) Read(p []byte) (int, error) {
	for len(d.buf) == 0 {
		switch {
		case d.stage == 0:
			d.buf, d.stage = []byte("["), 1
		case d.left > 0:
			d.buf = d.record
			d.left -= int64(len(d.buf))
		case d.stage == 1:
			d.buf, d.stage = []byte("null]"), 2
		default:
			return 0, io.EOF
		}
	}
	n := copy(p, d.buf)
	d.buf = d.buf[n:]
	return n, nil
}

func TestMemoryStaysFlatOnAThirtyTwoMiBDocument(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	record := `{"id":123456,"title":"` + strings.Repeat("lorem ipsum ", 40) + `","tags":["a","b","c"],"nested":{"x":1.5,"y":null}}`
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	n, err := Format(context.Background(), io.Discard, &docReader{left: 31 << 20, record: []byte(record + ",")}, Limits{})
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	alloc := after.TotalAlloc - before.TotalAlloc
	t.Logf("wrote %d MiB, allocated %d KiB", n>>20, alloc>>10)
	if alloc > 4<<20 {
		t.Fatalf("allocated %d KiB for a 31 MiB document", alloc>>10)
	}
}

func TestOneHugeStringIsStreamedNotBuffered(t *testing.T) {
	body := strings.Repeat("x", 20<<20)
	doc := `{"k":"` + body + `"}`
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	var sink countingWriter
	if _, err := Format(context.Background(), &sink, strings.NewReader(doc), Limits{}); err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&after)
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 4<<20 {
		t.Fatalf("a 20 MiB string cost %d KiB of allocation", alloc>>10)
	}
	if sink.n != int64(len(doc))+int64(len("{\n  \"k\": ")-len(`{"k":`))+int64(len("\n}")-len("}")) {
		t.Fatalf("wrote %d bytes for a %d byte document: the string was changed", sink.n, len(doc))
	}
}

type countingWriter struct{ n int64 }

func (c *countingWriter) Write(p []byte) (int, error) { c.n += int64(len(p)); return len(p), nil }

// ---- boundary ----

func TestImportsOnlyTheStandardLibrary(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range af.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if strings.Contains(strings.SplitN(path, "/", 2)[0], ".") {
				t.Errorf("%s imports %s: jsonview may use only the standard library", f, path)
			}
		}
	}
}

func TestAnIndentingBombIsRefusedBeforeItFillsTheDisk(t *testing.T) {
	// 100 levels deep, then a million one-digit elements: about 2 MB in, hundreds of MB out once every element is indented.
	doc := strings.Repeat("[", 100) + strings.Repeat("1,", 1_000_000) + "1" + strings.Repeat("]", 100)
	var sink countingWriter
	_, err := Format(context.Background(), &sink, strings.NewReader(doc), Limits{MaxOutputBytes: 8 << 20})
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("got %v", err)
	}
	if sink.n > 9<<20 {
		t.Fatalf("wrote %d MiB before stopping", sink.n>>20)
	}
	// a document within the output limit is untouched by it
	if _, err := Format(context.Background(), io.Discard, strings.NewReader(`[1,2,3]`), Limits{MaxOutputBytes: 1 << 10}); err != nil {
		t.Fatal(err)
	}
}
