package media

import (
	"io"
	"mime"
	"net/http"
	"os"
	"strings"
)

const sniffLen = 512

// ResolveContentType decides the content type stored for an uploaded file. A client's own declaration is kept when it says something specific; when it says
// nothing ("" or application/octet-stream, which is what a client sends when it does not know) the type is taken from the file itself: its first bytes
// (a PDF starts with "%PDF-"), then, where the bytes only say "text", "zip" or nothing at all, its extension. The stored type is what the download
// route sends back inline and what the Media page uses to choose a preview, so it must not be a guess the client made.
func ResolveContentType(declared, extension string, head []byte) string {
	if !isGeneric(declared) {
		return declared
	}
	sniffed := http.DetectContentType(head)
	byExt := mime.TypeByExtension(strings.ToLower(extension))
	if weak(sniffed) {
		if byExt != "" {
			return byExt
		}
		if sniffed != "" && !isGeneric(sniffed) {
			return sniffed
		}
		return "application/octet-stream"
	}
	return sniffed
}

func isGeneric(ct string) bool {
	base := strings.ToLower(strings.TrimSpace(strings.SplitN(ct, ";", 2)[0]))
	return base == "" || base == "application/octet-stream" || base == "binary/octet-stream"
}

// weak: what the first bytes say is too general to beat the extension.
func weak(sniffed string) bool {
	base := strings.ToLower(strings.TrimSpace(strings.SplitN(sniffed, ";", 2)[0]))
	return base == "application/octet-stream" || base == "text/plain" || base == "application/zip"
}

// ResolveStoredContentType reads the head of the file at path and resolves its content type (see ResolveContentType). A file that cannot be read keeps
// the declared type or the generic one.
func ResolveStoredContentType(declared, extension, path string) string {
	f, err := os.Open(path)
	if err != nil {
		if declared == "" {
			return "application/octet-stream"
		}
		return declared
	}
	defer f.Close()
	buf := make([]byte, sniffLen)
	n, _ := io.ReadFull(f, buf)
	return ResolveContentType(declared, extension, buf[:n])
}
