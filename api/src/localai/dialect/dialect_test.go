package dialect

import (
	"errors"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

type nop struct{}

func (nop) Encode(m []chatcompleter.Message, _ []chatcompleter.ToolDef) []chatcompleter.Message {
	return m
}
func (nop) Decode(*chatcompleter.ChatCompletionResult, []chatcompleter.ToolDef) {}

func TestARegistryHoldsDialectsByName(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.Lookup("a"); ok {
		t.Error("an empty registry knows nothing")
	}
	if err := r.Register("b-two", nop{}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register("a", nop{}); err != nil {
		t.Fatal(err)
	}
	if d, ok := r.Lookup("a"); !ok || d == nil {
		t.Error("a registered dialect must be found")
	}
	if got := strings.Join(r.Names(), ","); got != "a,b-two" {
		t.Errorf("names = %q, want them sorted", got)
	}
	var nilReg *Registry
	if _, ok := nilReg.Lookup("a"); ok {
		t.Error("no registry means no dialect")
	}
}

func TestARegistryRefusesWhatCannotWork(t *testing.T) {
	r := NewRegistry()
	if err := r.Register("a", nop{}); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		name string
		d    Dialect
	}{
		"a taken name": {"a", nop{}}, "no dialect": {"b", nil}, "no name": {"", nop{}},
		"upper case": {"Phi", nop{}}, "a space": {"a b", nop{}}, "a leading hyphen": {"-a", nop{}},
		"a trailing hyphen": {"a-", nop{}}, "a double hyphen": {"a--b", nop{}}, "a digit first": {"1a", nop{}},
		"too long": {"a" + strings.Repeat("b", MaxNameLength), nop{}},
	} {
		if err := r.Register(c.name, c.d); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	if got := len(r.Names()); got != 1 {
		t.Errorf("a refused registration must not be kept: %d names", got)
	}
}

func TestNameBounds(t *testing.T) {
	if !ValidName("phi-json") || !ValidName("a") || !ValidName("a1-b2") || !ValidName(strings.Repeat("a", MaxNameLength)) {
		t.Error("these are valid")
	}
	if ValidName(strings.Repeat("a", MaxNameLength+1)) {
		t.Error("33 characters is too long")
	}
}
