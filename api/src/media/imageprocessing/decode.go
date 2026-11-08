// Package imageprocessing is a plain image-in, image-out helper
// package with no knowledge of Media rows or the database at all —
// kept separate from media/persist.go's own file-handling code
// (persist.go copies arbitrary file bytes into permanent storage;
// this package only ever transforms already-decoded pixels). See
// plan/ai/media/step-02-media-schema-and-image-processing.md.
package imageprocessing

import (
	"fmt"
	"image"
	_ "image/gif" // decoder registration only — DecodeImage below never references the gif package directly
	_ "image/jpeg"
	_ "image/png"
	"io"
)

// DecodeImage sniffs the format (jpeg/png/gif — whatever the standard
// library's own registered decoders above support) and returns the
// decoded image plus the detected format name. Decoding
// attacker-controlled bytes relies entirely on the standard library's
// own decoders here — no custom parsing — see this package's own top
// doc comment and step-01's "Security considerations".
func DecodeImage(r io.Reader) (image.Image, string, error) {
	img, format, err := image.Decode(r)
	if err != nil {
		return nil, "", fmt.Errorf("not a decodable image: %w", err)
	}
	return img, format, nil
}
