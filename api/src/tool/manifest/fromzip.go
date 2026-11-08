package manifest

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
)

// MaxManifestBytes bounds a package's manifest.json: it is read into memory from an archive that came over the network.
const MaxManifestBytes = 1 << 20

// ReadFromZip reads and parses the manifest.json of a tool package without installing it or extracting anything (used to check a downloaded package
// against what a repository listed, and by repotool). Only that one entry is read, and at most MaxManifestBytes of it, whatever the archive claims.
func ReadFromZip(data []byte) (Manifest, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Manifest{}, fmt.Errorf("not a valid zip file: %w", err)
	}
	for _, f := range zr.File {
		if f.Name != "manifest.json" {
			continue
		}
		if f.UncompressedSize64 > MaxManifestBytes {
			return Manifest{}, fmt.Errorf("manifest.json is larger than %d bytes", MaxManifestBytes)
		}
		rc, err := f.Open()
		if err != nil {
			return Manifest{}, err
		}
		b, err := io.ReadAll(io.LimitReader(rc, MaxManifestBytes+1))
		rc.Close()
		if err != nil {
			return Manifest{}, err
		}
		if len(b) > MaxManifestBytes { // an archive that understates its size
			return Manifest{}, fmt.Errorf("manifest.json is larger than %d bytes", MaxManifestBytes)
		}
		return Parse(b)
	}
	return Manifest{}, fmt.Errorf("the package has no manifest.json")
}
