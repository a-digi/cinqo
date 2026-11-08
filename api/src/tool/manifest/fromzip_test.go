package manifest

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestReadFromZip(t *testing.T) {
	good := `{"slug":"career","name":"Career","version":"1.0.0","requires_tools":[{"slug":"browser","min_version":"1.0.0","max_version":"2.0.0"}]}`
	m, err := ReadFromZip(zipOf(t, map[string]string{"manifest.json": good, "frontend/bundle.js": "x"}))
	if err != nil || m.Slug != "career" || m.Version != "1.0.0" || len(m.RequiresTools) != 1 || m.RequiresTools[0].MaxVersion != "2.0.0" {
		t.Fatalf("%v %+v", err, m)
	}
	for name, tc := range map[string]struct {
		data []byte
		want string
	}{
		"not a zip":         {[]byte("plain text"), "not a valid zip"},
		"no manifest":       {zipOf(t, map[string]string{"frontend/bundle.js": "x"}), "no manifest.json"},
		"only a nested one": {zipOf(t, map[string]string{"sub/manifest.json": good}), "no manifest.json"},
		"bad json":          {zipOf(t, map[string]string{"manifest.json": "{not json"}), ""},
		"too large":         {zipOf(t, map[string]string{"manifest.json": `{"slug":"x","name":"` + strings.Repeat("a", MaxManifestBytes) + `"}`}), "larger than"},
		"empty archive":     {zipOf(t, map[string]string{}), "no manifest.json"},
	} {
		_, err := ReadFromZip(tc.data)
		if err == nil || (tc.want != "" && !strings.Contains(err.Error(), tc.want)) {
			t.Errorf("%s: err = %v, want it to mention %q", name, err, tc.want)
		}
	}
	// exactly at the limit is read (the JSON is padded with spaces)
	pad := good + strings.Repeat(" ", MaxManifestBytes-len(good))
	if _, err := ReadFromZip(zipOf(t, map[string]string{"manifest.json": pad})); err != nil {
		t.Errorf("a manifest of exactly the limit is read: %v", err)
	}
}
