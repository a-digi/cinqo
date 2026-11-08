package cv

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// tinyPDF (a copy of pdftext's test helper) builds a small valid one-page PDF whose text is the given lines (Helvetica, one per line, top down); no lines gives an empty page.
func tinyPDF(lines ...string) []byte {
	var content bytes.Buffer
	if len(lines) > 0 {
		content.WriteString("BT /F1 12 Tf 14 TL 72 740 Td\n")
		for _, l := range lines {
			l = strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`).Replace(l)
			fmt.Fprintf(&content, "(%s) Tj T*\n", l)
		}
		content.WriteString("ET\n")
	}
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", content.Len(), content.String()),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for i, o := range objects {
		offsets[i] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, off := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return out.Bytes()
}

const textFileID = "6f1f2c0e-9c0b-4a39-8f43-3b1f2a7d9e10"

// core starts a fake core whose download answers with the given status and body, and records what it was asked.
func core(t *testing.T, status int, body []byte) (*httptest.Server, *[]*http.Request) {
	t.Helper()
	var mu sync.Mutex
	var seen []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r)
		mu.Unlock()
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CORE_API_URL", srv.URL)
	return srv, &seen
}

func getText(target string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	CVImportTextHandler(rec, req)
	return rec
}

func TestTheTextRouteFetchesTheFileAsTheCallerAndGivesItsText(t *testing.T) {
	_, seen := core(t, http.StatusOK, tinyPDF("Andet Tafa", "Berufserfahrung", "Abschlüsse"))
	rec := getText("/cv-import/text?fileId="+textFileID, map[string]string{"Authorization": "Bearer abc", "Cookie": "s=1"})
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	got := decode(t, rec)
	text, _ := got["text"].(string)
	if !strings.Contains(text, "Andet Tafa") || !strings.Contains(text, "Berufserfahrung") || got["characters"] != float64(len([]rune(text))) {
		t.Errorf("%v", got)
	}
	if len(*seen) != 1 {
		t.Fatalf("%d requests to the core", len(*seen))
	}
	r := (*seen)[0]
	if r.URL.Path != "/api/v1/media/"+textFileID+"/download" || r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer abc" || r.Header.Get("Cookie") != "s=1" {
		t.Errorf("the core was asked %s %s with %v", r.Method, r.URL.Path, r.Header)
	}
}

func TestTheTextRouteSendsNoLoginWhenTheCallerHasNone(t *testing.T) {
	_, seen := core(t, http.StatusOK, tinyPDF("x"))
	getText("/cv-import/text?fileId="+textFileID, nil)
	if r := (*seen)[0]; r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
		t.Errorf("%v", r.Header)
	}
}

func TestTheTextRouteSaysWhatWentWrong(t *testing.T) {
	cases := map[string]struct {
		status int
		body   []byte
		want   int
	}{
		"not found":           {http.StatusNotFound, nil, http.StatusNotFound},
		"forbidden":           {http.StatusForbidden, nil, http.StatusNotFound},
		"not logged in":       {http.StatusUnauthorized, nil, http.StatusNotFound},
		"core error":          {http.StatusInternalServerError, nil, http.StatusBadGateway},
		"not a pdf":           {http.StatusOK, []byte("a letter"), http.StatusUnprocessableEntity},
		"a pdf without text":  {http.StatusOK, tinyPDF(), http.StatusUnprocessableEntity},
		"larger than allowed": {http.StatusOK, bytes.Repeat([]byte("a"), maxCVUploadBytes+10), http.StatusBadGateway},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			core(t, c.status, c.body)
			if rec := getText("/cv-import/text?fileId="+textFileID, nil); rec.Code != c.want {
				t.Errorf("%d %s, want %d", rec.Code, rec.Body.String(), c.want)
			}
		})
	}
	t.Run("core unreachable", func(t *testing.T) {
		srv, _ := core(t, http.StatusOK, nil)
		srv.Close()
		if rec := getText("/cv-import/text?fileId="+textFileID, nil); rec.Code != http.StatusBadGateway {
			t.Errorf("%d", rec.Code)
		}
	})
}

func TestTheTextRouteNeedsAFileIdAndOnlyAnswersGET(t *testing.T) {
	_, seen := core(t, http.StatusOK, tinyPDF("x"))
	for _, target := range []string{"/cv-import/text", "/cv-import/text?fileId=", "/cv-import/text?fileId=r1", "/cv-import/text?fileId=../../etc"} {
		if rec := getText(target, nil); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", target, rec.Code)
		}
	}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		CVImportTextHandler(rec, httptest.NewRequest(method, "/cv-import/text?fileId="+textFileID, nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: %d", method, rec.Code)
		}
	}
	if len(*seen) != 0 {
		t.Errorf("a refused request must not reach the core: %d", len(*seen))
	}
}
