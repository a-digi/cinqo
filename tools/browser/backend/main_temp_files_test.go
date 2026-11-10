package main

import (
	"os"
	"strings"
	"testing"
)

// The temporary files folder is cleared at every start of the tool, after the Chrome sweep and before the first request is served, and at a clean stop,
// and never by the short-lived --mcp process (plan 248, step 1).
func TestTheTemporaryFilesAreClearedAtStartAndStopAndNotByTheMCPProcess(t *testing.T) {
	raw, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	idx := func(s string) int {
		i := strings.Index(src, s)
		if i < 0 {
			t.Fatalf("main.go no longer contains %q", s)
		}
		return i
	}
	sweep, start, listen := idx("chromeproc.Sweep()"), idx(`SummarizeTempClear("at start"`), idx("http.ListenAndServe(")
	if !(sweep < start && start < listen) {
		t.Errorf("the start-up clear must come after the Chrome sweep (%d) and before the server listens (%d), it is at %d", sweep, listen, start)
	}
	stop := idx(`SummarizeTempClear("at stop"`)
	handler := strings.LastIndex(src[:stop], "<-sigCh")
	if handler < 0 || strings.Index(src[handler:], "crawler.StopKatanaWindow()") < 0 || strings.Index(src[handler:], "crawler.StopKatanaWindow()") > stop-handler {
		t.Error("the stop clear must come in the shutdown handler after the engines are stopped")
	}
	if exit := strings.Index(src[stop:], "os.Exit(0)"); exit < 0 || exit > 200 {
		t.Error("the stop clear must come right before the process exits")
	}
	if i := strings.Index(src, "func runMCPServer()"); i < 0 || strings.Contains(src[i:], "ClearTemporaryFiles") {
		t.Error("the --mcp process shares the folder with the running tool and must never clear it")
	}
	if !strings.Contains(src, `http.HandleFunc("/temp-files", crawler.TempFilesHandler)`) {
		t.Error("the route is not registered")
	}
	mf, err := os.ReadFile("../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{`"method": "GET",` + "\n" + `      "path_suffix": "temp-files"`, `"method": "DELETE",` + "\n" + `      "path_suffix": "temp-files"`} {
		if !strings.Contains(string(mf), m) {
			t.Errorf("manifest.json lacks the route %q", m)
		}
	}
}
