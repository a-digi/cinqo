package localai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The tests' own binary doubles as the model program: started with FAKE_LLAMA=1 it behaves like a llama-server (answering
// /health and /v1/chat/completions on the port it is given, requiring the key from LLAMA_API_KEY), steered by FAKE_LLAMA_*
// variables. This keeps the supervisor tests on real processes, real ports and real signals.

func TestMain(m *testing.M) {
	if os.Getenv("FAKE_LLAMA") == "1" {
		runFakeLlama()
		return
	}
	os.Exit(m.Run())
}

func runFakeLlama() {
	args := os.Args[1:]
	arg := func(name string) string {
		for i, a := range args {
			if a == name && i+1 < len(args) {
				return args[i+1]
			}
		}
		return ""
	}
	if f := os.Getenv("FAKE_LLAMA_RECORD"); f != "" {
		var env []string
		for _, e := range os.Environ() {
			env = append(env, strings.SplitN(e, "=", 2)[0])
		}
		raw, _ := json.Marshal(map[string]any{"args": args, "envNames": env})
		_ = os.WriteFile(f, raw, 0o600)
	}
	mode := os.Getenv("FAKE_LLAMA_MODE")
	if mode == "exit" {
		fmt.Fprintln(os.Stderr, "0.00.106.893 E llama_model_load: error loading model: tensor 'token_embd.weight' data is not within the file bounds, model is corrupted or incomplete")
		os.Exit(3)
	}
	delay, _ := strconv.Atoi(os.Getenv("FAKE_LLAMA_DELAY_MS"))
	chatMs, _ := strconv.Atoi(os.Getenv("FAKE_LLAMA_CHAT_MS"))
	crashMs, _ := strconv.Atoi(os.Getenv("FAKE_LLAMA_CRASH_MS"))
	key := os.Getenv("LLAMA_API_KEY")
	started := time.Now()

	var inFlight, maxInFlight, requests atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if mode == "never" || time.Since(started) < time.Duration(delay)*time.Millisecond {
			http.Error(w, `{"error":{"message":"Loading model"}}`, http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+key {
			http.Error(w, `{"error":{"message":"Invalid API Key"}}`, http.StatusUnauthorized)
			return
		}
		n := inFlight.Add(1)
		for {
			m := maxInFlight.Load()
			if n <= m || maxInFlight.CompareAndSwap(m, n) {
				break
			}
		}
		requests.Add(1)
		time.Sleep(time.Duration(chatMs) * time.Millisecond)
		inFlight.Add(-1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":4,"total_tokens":16}}`))
	})
	mux.HandleFunc("/_stats", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]int64{"maxInFlight": maxInFlight.Load(), "requests": requests.Load()})
	})
	if crashMs > 0 {
		go func() {
			time.Sleep(time.Duration(delay+crashMs) * time.Millisecond)
			fmt.Fprintln(os.Stderr, "0.00.500.000 E srv  fatal: out of memory")
			os.Exit(7)
		}()
	}
	if err := http.ListenAndServe("127.0.0.1:"+arg("--port"), mux); err != nil {
		fmt.Fprintln(os.Stderr, "listen:", err)
		os.Exit(4)
	}
}

// fakeCommand returns a command factory that starts the stand-in with the given FAKE_LLAMA_* settings, and counts the starts.
func fakeCommand(starts *atomic.Int32, env ...string) func(string, []string) *exec.Cmd {
	return func(_ string, args []string) *exec.Cmd {
		starts.Add(1)
		cmd := exec.Command(os.Args[0], args...)
		cmd.Env = append([]string{"FAKE_LLAMA=1"}, env...)
		return cmd
	}
}

// quick makes the supervisor's clocks short enough for tests.
func quick(o *supervisorOptions) {
	o.HealthEvery = 15 * time.Millisecond
	o.Grace = 500 * time.Millisecond
	o.StartTimeout = 10 * time.Second
	o.Cooldown = 400 * time.Millisecond
	o.IdleUnit = 60 * time.Millisecond
}

func testSpec(t *testing.T) launch {
	t.Helper()
	dir := t.TempDir()
	return launch{Binary: os.Args[0], ModelPath: dir + "/model.gguf", ModelID: "tiny", ContextTokens: 4096, Dir: dir}
}

var httpGet = &http.Client{Timeout: 5 * time.Second}

func stats(t *testing.T, baseURL, token string) map[string]int64 {
	t.Helper()
	req, _ := http.NewRequest("GET", strings.TrimSuffix(baseURL, "/v1")+"/_stats", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := httpGet.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]int64
	json.NewDecoder(resp.Body).Decode(&out)
	return out
}

func postChat(baseURL, token string) (int, error) {
	req, _ := http.NewRequest("POST", baseURL+"/chat/completions", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := httpGet.Do(req)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return resp.StatusCode, nil
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func gone(pid int) bool { return !processAlive(pid) }

var _ = sync.Mutex{}
var _ = testing.Short
