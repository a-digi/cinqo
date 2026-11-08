package pdftext

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// MakePDF builds a small valid one-page PDF whose text is the given lines (Helvetica, one per line, top down); no lines gives an empty page.
func MakePDF(lines ...string) []byte {
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

func TestExtractTextGivesTheCleanedTextOfAPDF(t *testing.T) {
	text, err := ExtractText(MakePDF("Andet Tafa", "Berufserfahrung", "VERSO GmbH Team Lead 09/2024 - aktuell"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Andet Tafa", "Berufserfahrung", "VERSO GmbH Team Lead 09/2024 - aktuell"} {
		if !strings.Contains(text, want) {
			t.Errorf("%q is missing from %q", want, text)
		}
	}
	if text != CleanText(text) {
		t.Errorf("the text must already be cleaned: %q", text)
	}
}

func TestExtractTextOfAPDFWithoutTextIsErrNoText(t *testing.T) {
	if _, err := ExtractText(MakePDF()); !errors.Is(err, ErrNoText) {
		t.Errorf("err = %v, want ErrNoText", err)
	}
}

func TestExtractTextOfNotAPDFIsAnErrorThatIsNotErrNoText(t *testing.T) {
	for name, data := range map[string][]byte{"text": []byte("just a letter"), "empty": nil, "cut": MakePDF("x")[:40]} {
		_, err := ExtractText(data)
		if err == nil || errors.Is(err, ErrNoText) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}
