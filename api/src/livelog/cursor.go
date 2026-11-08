package livelog

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
)

// Cursor says where a follower stopped: the byte offset after the last line it returned, and the log's identity at that moment (what
// the provider uses to notice that the log was replaced or truncated: size, modification time, inode, whatever it has). It carries no
// secret and grants nothing: it is an offset against a log the server resolves itself, so it is not signed.
type Cursor struct {
	Offset   int64  `json:"o"`
	Identity string `json:"i,omitempty"`
}

const (
	// maxCursorText bounds the encoded text a client may send.
	maxCursorText = 512
	// MaxIdentityBytes bounds what a provider may put in Identity.
	MaxIdentityBytes = 200
)

// Encode is the opaque text of the cursor: URL-safe base64 without padding of its JSON.
func (c Cursor) Encode() string {
	raw, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(raw)
}

// ParseCursor reads what a client sent. Empty text means "no cursor" (present false). Anything else must be exactly what Encode
// produces: strict base64, one JSON object with only the known fields, a non-negative offset and a bounded identity; every other
// text is ErrBadCursor.
func ParseCursor(text string) (c Cursor, present bool, err error) {
	if text == "" {
		return Cursor{}, false, nil
	}
	if len(text) > maxCursorText {
		return Cursor{}, false, ErrBadCursor
	}
	// the base64 decoder skips line breaks silently: a cursor never has one
	if strings.ContainsAny(text, "\r\n") {
		return Cursor{}, false, ErrBadCursor
	}
	raw, derr := base64.RawURLEncoding.Strict().DecodeString(text)
	if derr != nil {
		return Cursor{}, false, ErrBadCursor
	}
	// one JSON object: "null" and the like decode into a struct without an error
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '{' {
		return Cursor{}, false, ErrBadCursor
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if derr := dec.Decode(&c); derr != nil {
		return Cursor{}, false, ErrBadCursor
	}
	if dec.More() { // anything after the object
		return Cursor{}, false, ErrBadCursor
	}
	if c.Offset < 0 || len(c.Identity) > MaxIdentityBytes {
		return Cursor{}, false, ErrBadCursor
	}
	return c, true, nil
}
