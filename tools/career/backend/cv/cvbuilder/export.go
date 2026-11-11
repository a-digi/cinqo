package cvbuilder

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode"
)

// maxExportName is how many characters of a CV's title go into the name of the file that is downloaded.
const maxExportName = 100

// exportBaseName is the title of a CV as a file name: letters, digits, spaces, dashes and underscores only (no path, no quote, no line break can reach the header),
// spaces collapsed, at most maxExportName characters, and "cv" when nothing is left.
func exportBaseName(title string) string {
	var b strings.Builder
	for _, r := range title {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_':
			b.WriteRune(r)
		case unicode.IsSpace(r) || r == ' ':
			b.WriteRune(' ')
		}
	}
	name := strings.Join(strings.Fields(b.String()), " ")
	if r := []rune(name); len(r) > maxExportName {
		name = strings.TrimSpace(string(r[:maxExportName]))
	}
	if name == "" {
		return "cv"
	}
	return name
}

// exportContentDisposition is an attachment header with a plain ASCII name for old clients and the real name (RFC 5987) for the others.
func exportContentDisposition(title, ext string) string {
	base := exportBaseName(title)
	ascii := strings.Map(func(r rune) rune {
		if r > 0x7e {
			return '_'
		}
		return r
	}, base)
	return fmt.Sprintf(`attachment; filename="%s.%s"; filename*=UTF-8''%s.%s`, ascii, ext, url.PathEscape(base), ext)
}

// docxContentType is the media type of a Word (.docx) file.
const docxContentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

// ExportHandler handles GET cv-documents/export?cvDocumentId=<id>&format=md|docx (plan 251, steps 5 and 6): a saved CV as a Markdown or a Word file, made on the spot
// from the CV's own data and streamed as a download. Nothing is stored. An unknown id is a 404, a missing id or a format that is not offered a 400.
func ExportHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("cvDocumentId")
	if id == "" {
		http.Error(w, "cvDocumentId query parameter is required", http.StatusBadRequest)
		return
	}
	format := r.URL.Query().Get("format")
	if format != "md" && format != "docx" {
		http.Error(w, `format must be "md" or "docx"`, http.StatusBadRequest)
		return
	}
	doc, err := GetCvDocument(id)
	if err != nil {
		writeCvDocumentAwareError(w, "read cv document", err)
		return
	}
	var body []byte
	contentType := "text/markdown; charset=utf-8"
	if format == "docx" {
		contentType = docxContentType
		if body, err = RenderDocx(doc.Data); err != nil {
			http.Error(w, "failed to make the Word file", http.StatusInternalServerError)
			return
		}
	} else {
		body = []byte(RenderMarkdown(doc.Data))
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", exportContentDisposition(doc.Title, format))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(body)
}
