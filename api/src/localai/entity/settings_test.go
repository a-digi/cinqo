package entity

import (
	"errors"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/localai/catalog"
)

const (
	phi  = "phi-4-mini-instruct-q4_k_m"   // reads up to 131,072 tokens
	qwen = "qwen2.5-1.5b-instruct-q4_k_m" // reads up to 32,768 tokens
)

func shipped(t *testing.T) catalog.Catalog {
	t.Helper()
	c, err := catalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func base() Settings { return Settings{ModelID: phi, ContextTokens: 4096, IdleMinutes: 10} }

var errNotBuilt = errors.New("the managed runtime is not available yet")

func TestAValidChangeIsAcceptedAndNormalised(t *testing.T) {
	in := base()
	in.Enabled = true
	in.ModelID = "  " + qwen + "  "
	got, err := Validate(in, shipped(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.ModelID != qwen || got.ID != 1 || !got.Enabled || !got.Configured() {
		t.Errorf("got %+v, want a trimmed model id, id 1, enabled and configured", got)
	}
}

func TestSwitchedOffSettingsMayHaveNoModel(t *testing.T) {
	in := base()
	in.ModelID = ""
	got, err := Validate(in, shipped(t), errNotBuilt)
	if err != nil {
		t.Fatalf("a switched-off row with no model must be saved: %v", err)
	}
	if got.Configured() {
		t.Error("a switched-off row is never configured")
	}
}

func TestAModelCanBeChosenWhileSwitchedOffEvenIfTheRuntimeIsNotThere(t *testing.T) {
	in := base()
	if _, err := Validate(in, shipped(t), errNotBuilt); err != nil {
		t.Errorf("choosing a model while off must work before the runtime exists: %v", err)
	}
}

func TestSwitchingOnIsRefusedWhileTheRuntimeCannotRun(t *testing.T) {
	in := base()
	in.Enabled = true
	_, err := Validate(in, shipped(t), errNotBuilt)
	if !errors.Is(err, ErrInvalidSettings) || err.Error() != "invalid local AI settings: the managed runtime is not available yet" {
		t.Errorf("err = %v, want the reason of the runtime", err)
	}
}

func TestSwitchingOnNeedsAModel(t *testing.T) {
	in := base()
	in.Enabled = true
	in.ModelID = "   "
	if _, err := Validate(in, shipped(t), nil); !errors.Is(err, ErrInvalidSettings) {
		t.Errorf("err = %v, want ErrInvalidSettings", err)
	}
}

func TestOnlyACatalogModelCanBeChosen(t *testing.T) {
	for _, id := range []string{"gpt-4", "phi-4-mini-instruct", "PHI-4-MINI-INSTRUCT-Q4_K_M", "../etc/passwd", "http://127.0.0.1:8080", phi + "x", "a\nb"} {
		in := base()
		in.ModelID = id
		_, err := Validate(in, shipped(t), nil)
		if !errors.Is(err, ErrInvalidSettings) || !strings.Contains(err.Error(), "one of the models Cinqo offers") {
			t.Errorf("%q: err = %v, want the catalog refusal", id, err)
		}
	}
}

func TestTheContextMayNotExceedWhatTheChosenModelCanRead(t *testing.T) {
	c := shipped(t)
	in := base()
	in.ModelID = qwen
	in.ContextTokens = 32768
	if _, err := Validate(in, c, nil); err != nil {
		t.Errorf("exactly the model's context must be accepted: %v", err)
	}
	in.ContextTokens = 32769
	if _, err := Validate(in, c, nil); !errors.Is(err, ErrInvalidSettings) {
		t.Errorf("one token more than qwen reads: err = %v, want ErrInvalidSettings", err)
	}
	in.ModelID = phi
	if _, err := Validate(in, c, nil); err != nil {
		t.Errorf("the same context is fine for phi, which reads more: %v", err)
	}
	// With no model chosen only the global bounds apply.
	in.ModelID = ""
	in.ContextTokens = MaxContextTokens
	if _, err := Validate(in, c, nil); err != nil {
		t.Errorf("no model, global bound: %v", err)
	}
}

func TestTheNumericBounds(t *testing.T) {
	c := shipped(t)
	bad := map[string]func(*Settings){
		"too small context": func(s *Settings) { s.ContextTokens = MinContextTokens - 1 },
		"too large context": func(s *Settings) { s.ModelID = ""; s.ContextTokens = MaxContextTokens + 1 },
		"negative idle":     func(s *Settings) { s.IdleMinutes = -1 },
		"too long idle":     func(s *Settings) { s.IdleMinutes = MaxIdleMinutes + 1 },
	}
	for name, mutate := range bad {
		in := base()
		mutate(&in)
		if _, err := Validate(in, c, nil); !errors.Is(err, ErrInvalidSettings) {
			t.Errorf("%s: err = %v, want ErrInvalidSettings", name, err)
		}
	}
	for _, tokens := range []int{MinContextTokens, 32768} {
		in := base()
		in.ContextTokens = tokens
		if _, err := Validate(in, c, nil); err != nil {
			t.Errorf("contextTokens %d must be accepted: %v", tokens, err)
		}
	}
	for _, idle := range []int{0, MaxIdleMinutes} {
		in := base()
		in.IdleMinutes = idle
		if _, err := Validate(in, c, nil); err != nil {
			t.Errorf("idleMinutes %d must be accepted: %v", idle, err)
		}
	}
}

func TestConfiguredNeedsSwitchedOnAndAModel(t *testing.T) {
	on := Settings{Enabled: true, ModelID: phi}
	if !on.Configured() {
		t.Fatal("on with a model must be configured")
	}
	for name, s := range map[string]Settings{"off": {ModelID: phi}, "no model": {Enabled: true}, "neither": {}} {
		if s.Configured() {
			t.Errorf("%s must not be configured", name)
		}
	}
}
