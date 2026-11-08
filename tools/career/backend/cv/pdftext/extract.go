package pdftext

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/ledongthuc/pdf"
)

// ErrNoText says the PDF opened but holds no text (a scan, an image-only design): nothing for a reader of text to work on.
var ErrNoText = errors.New("the PDF holds no readable text")

// ExtractText is the cleaned text of a PDF, as the pdf_to_markdown tool gives it to a model (the same library and the same CleanText; see
// tools/pdf_tools/backend/convert.extractText). It reads from memory: pdf.NewReader takes any io.ReaderAt.
func ExtractText(pdfBytes []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(pdfBytes), int64(len(pdfBytes)))
	if err != nil {
		return "", fmt.Errorf("failed to open pdf: %w", err)
	}
	reader, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("failed to extract text: %w", err)
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(reader); err != nil {
		return "", fmt.Errorf("failed to read extracted text: %w", err)
	}
	text := CleanText(buf.String())
	if text == "" {
		return "", ErrNoText
	}
	return text, nil
}
