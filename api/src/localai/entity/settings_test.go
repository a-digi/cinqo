package entity

import (
	"errors"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/localai/catalog"
	"github.com/a-digi/cinqo/src/localai/hardware"
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

const gib16 = uint64(16) << 30

func TestAContextSizeThePhiMachineCannotHoldIsRefusedWithTheNumbers(t *testing.T) {
	cat := shipped(t)
	for _, c := range []struct {
		name   string
		model  string
		tokens int
		ram    uint64
		want   string // "" = accepted
	}{
		{"the default on a 16 GiB machine", phi, 4096, gib16, ""},
		{"the import's size on 16 GiB", phi, 16384, gib16, ""},
		{"the largest on 16 GiB that fits", phi, 65536, gib16, ""},
		{"128K on 16 GiB needs 21 GiB", phi, 131072, gib16, "a context of 131,072 tokens needs about 21.4 GiB of memory (the model, its cache and a reserve for the system); this computer has 16.0 GiB. Choose at most 65,536 tokens"},
		{"16K on an 8 GiB machine fits", phi, 16384, 8 << 30, ""},
		{"32K on an 8 GiB machine does not", phi, 32768, 8 << 30, "Choose at most 16,384 tokens"},
		// A machine that cannot hold the model at all is the business of the "needs more memory" rule, not of the context check:
		// choosing such a model while the local AI is off keeps working.
		{"not even the model fits at the baseline", phi, 4096, 5 << 30, ""},
		{"not even the model fits, a larger context", phi, 8192, 5 << 30, ""},
		{"qwen 1.5B 32K on 8 GiB is small", qwen, 32768, 8 << 30, ""},
		{"unknown memory is never held against the admin", phi, 131072, 0, ""},
		{"no model chosen yet", "", 131072, 4 << 30, ""},
	} {
		s := Settings{ModelID: c.model, ContextTokens: c.tokens, IdleMinutes: 10}
		err := CheckMemory(s, cat, c.ram)
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: refused: %v", c.name, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%s: %v, want it to contain %q", c.name, err, c.want)
		case c.want != "" && !errors.Is(err, ErrInvalidSettings):
			t.Errorf("%s: must be an invalid-settings error: %v", c.name, err)
		}
	}
}

func TestTheMemoryCheckOnlyOffersSizesTheModelCanRead(t *testing.T) {
	// Qwen 1.5B reads at most 32,768; on a machine that could hold more, the suggestion never goes past what it can read.
	cat := shipped(t)
	m, _ := cat.Find(qwen)
	tiny := hardware16(m.SizeBytes) // just enough for 32K with room to spare
	if err := CheckMemory(Settings{ModelID: qwen, ContextTokens: 32768}, cat, tiny); err != nil {
		t.Fatal(err)
	}
	got := SizesUpTo(32768)
	if got[len(got)-1] != 32768 || len(got) != 5 {
		t.Errorf("SizesUpTo(32768) = %v", got)
	}
	if all := SizesUpTo(0); len(all) != len(ContextSizes) {
		t.Errorf("an unknown maximum offers everything: %v", all)
	}
	if one := SizesUpTo(1000); len(one) != 0 {
		t.Errorf("a maximum below the smallest size offers nothing: %v", one)
	}
}

func hardware16(model int64) uint64 { return uint64(model)*5/4 + (3 << 30) + (1 << 30) }

func TestTheOfferedSizesAreAscending(t *testing.T) {
	for i := 1; i < len(ContextSizes); i++ {
		if ContextSizes[i] <= ContextSizes[i-1] {
			t.Errorf("ContextSizes must ascend: %v", ContextSizes)
		}
	}
	if ContextSizes[0] < MinContextTokens || ContextSizes[len(ContextSizes)-1] > MaxContextTokens {
		t.Errorf("every offered size must be a valid setting: %v", ContextSizes)
	}
}

func TestTheMemoryCheckAcceptsExactlyEnoughAndRefusesOneByteLess(t *testing.T) {
	cat := shipped(t)
	m, _ := cat.Find(phi)
	s := Settings{ModelID: phi, ContextTokens: 16384, IdleMinutes: 10}
	need := hardware.RequiredBytes(m.SizeBytes, m.KVBytesPerToken, 16384)
	if err := CheckMemory(s, cat, need); err != nil {
		t.Errorf("exactly the memory a size needs must be accepted: %v", err)
	}
	if err := CheckMemory(s, cat, need-1); err == nil {
		t.Error("one byte less must be refused")
	}
}
