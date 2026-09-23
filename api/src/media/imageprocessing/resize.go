package imageprocessing

import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"

	"github.com/disintegration/imaging"
)

// ResizeToMax scales img down so neither dimension exceeds maxDim,
// preserving aspect ratio — a no-op (returns img unchanged) if it
// already fits either dimension within the cap, and NEVER upscales a
// smaller image. maxDim is the longest-edge cap from
// plan/ai/media/step-01-image-upload-crop-resize-overview.md's own
// Open Question 5 (1600, as approved).
func ResizeToMax(img image.Image, maxDim int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxDim && h <= maxDim {
		return img
	}
	if w >= h {
		return imaging.Resize(img, maxDim, 0, imaging.Lanczos)
	}
	return imaging.Resize(img, 0, maxDim, imaging.Lanczos)
}

// EncodeImage writes img in the given format ("jpeg" or anything
// else, which becomes "jpeg" too — the one format this function
// itself treats specially is "png", per step 1's own
// format-preservation rule: PNG in, PNG out, to keep transparency;
// everything else in, JPEG out at jpegQuality (1-100)).
func EncodeImage(w io.Writer, img image.Image, format string, jpegQuality int) error {
	if format == "png" {
		if err := png.Encode(w, img); err != nil {
			return fmt.Errorf("failed to encode png: %w", err)
		}
		return nil
	}
	if err := jpeg.Encode(w, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return fmt.Errorf("failed to encode jpeg: %w", err)
	}
	return nil
}

// OutputFormat applies step 1's own format-preservation rule to a
// DecodeImage-reported source format, returning the format
// EncodeImage should be called with.
func OutputFormat(sourceFormat string) string {
	if sourceFormat == "png" {
		return "png"
	}
	return "jpeg"
}
