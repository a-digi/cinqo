package chatcompleter

import (
	"math"
	"testing"
)

func f64(v float64) *float64 { return &v }

func TestTheUsageObjectIsFoundOnlyWhenItIsAnObject(t *testing.T) {
	if m := UsageObject([]byte(`{"choices":[],"usage":{"prompt_tokens":5,"cost":0.01}}`)); m == nil || m["prompt_tokens"].(float64) != 5 {
		t.Errorf("a usage object: %v", m)
	}
	for name, body := range map[string]string{
		"missing":        `{"choices":[]}`,
		"null":           `{"usage":null}`,
		"a text":         `{"usage":"lots"}`,
		"a list":         `{"usage":[1,2]}`,
		"a number":       `{"usage":7}`,
		"not json":       `nope`,
		"empty":          ``,
		"an empty usage": `{"usage":{}}`,
	} {
		m := UsageObject([]byte(body))
		if name == "an empty usage" {
			if m == nil || len(m) != 0 {
				t.Errorf("%s: %v", name, m)
			}
			continue
		}
		if m != nil {
			t.Errorf("%s: %v, want nil", name, m)
		}
	}
}

func TestCountsAreReadLeniently(t *testing.T) {
	m := UsageObject([]byte(`{"usage":{"a":12,"b":"34","c":" 5 ","d":7.9,"e":-3,"f":null,"g":"abc","h":true,"i":{"x":1},"j":[1],"k":"NaN","l":99999999999,
		"details":{"cached_tokens":8000,"deep":{"n":2}},"flat":4}}`))
	cases := []struct {
		path []string
		want int
	}{
		{[]string{"a"}, 12}, {[]string{"b"}, 34}, {[]string{"c"}, 5}, {[]string{"d"}, 7}, {[]string{"e"}, 0}, {[]string{"f"}, 0},
		{[]string{"g"}, 0}, {[]string{"h"}, 0}, {[]string{"i"}, 0}, {[]string{"j"}, 0}, {[]string{"k"}, 0}, {[]string{"l"}, 0},
		{[]string{"details", "cached_tokens"}, 8000}, {[]string{"details", "deep", "n"}, 2},
		{[]string{"missing"}, 0}, {[]string{"details", "missing"}, 0}, {[]string{"flat", "x"}, 0}, {[]string{"details", "cached_tokens", "x"}, 0},
		{nil, 0},
	}
	for _, c := range cases {
		if got := IntAt(m, c.path...); got != c.want {
			t.Errorf("IntAt(%v) = %d, want %d", c.path, got, c.want)
		}
	}
	if IntAt(nil, "a") != 0 {
		t.Error("a nil usage reads 0")
	}
}

func TestACostIsNilWhenUnknownAndZeroWhenFree(t *testing.T) {
	m := UsageObject([]byte(`{"usage":{"cost":0.000123,"free":0,"text":"0.5","neg":-0.1,"null":null,"bad":"cheap","obj":{},"nested":{"cost":2},"nan":"NaN","inf":"+Inf","ninf":"-Inf"}}`))
	if got := CostAt(m, "cost"); got == nil || *got != 0.000123 {
		t.Errorf("cost = %v", got)
	}
	if got := CostAt(m, "free"); got == nil || *got != 0 {
		t.Errorf("a free call must be 0, not unknown: %v", got)
	}
	if got := CostAt(m, "text"); got == nil || *got != 0.5 {
		t.Errorf("a text of a number: %v", got)
	}
	if got := CostAt(m, "nested", "cost"); got == nil || *got != 2 {
		t.Errorf("a nested cost: %v", got)
	}
	for _, key := range []string{"neg", "null", "bad", "obj", "missing", "nan", "inf", "ninf"} {
		if got := CostAt(m, key); got != nil {
			t.Errorf("%s: %v, want nil", key, *got)
		}
	}
	// a path that runs into something that is not an object is "not there", never a zero cost
	if got := CostAt(m, "free", "deeper"); got != nil {
		t.Errorf("a path through a number: %v", *got)
	}
	if got := CostAt(m, "text", "deeper"); got != nil {
		t.Errorf("a path through a text: %v", *got)
	}
	if CostAt(nil, "cost") != nil {
		t.Error("no usage, no cost")
	}
	if _, ok := numberAt(map[string]any{"x": math.Inf(1)}, "x"); ok {
		t.Error("an infinite value is not a number")
	}
}

func TestAddingUsageSumsCountsAndOnlyTheCostsThatWereReported(t *testing.T) {
	var total Usage
	total.Add(Usage{PromptTokens: 100, CompletionTokens: 10, TotalTokens: 110, CacheReadTokens: 60, CacheCreationTokens: 5, ReasoningTokens: 4})
	if total.Cost != nil {
		t.Fatalf("no call reported a cost: %v", *total.Cost)
	}
	total.Add(Usage{PromptTokens: 50, CompletionTokens: 5, TotalTokens: 55, CacheReadTokens: 10, ReasoningTokens: 1, Cost: f64(0.25)})
	total.Add(Usage{PromptTokens: 1, Cost: f64(0)})
	total.Add(Usage{PromptTokens: 2}) // a call without a cost keeps the sum and does not turn it unknown
	want := Usage{PromptTokens: 153, CompletionTokens: 15, TotalTokens: 165, CacheReadTokens: 70, CacheCreationTokens: 5, ReasoningTokens: 5}
	got := total
	got.Cost = nil
	if got != want {
		t.Errorf("counts = %+v, want %+v", got, want)
	}
	if total.Cost == nil || *total.Cost != 0.25 {
		t.Errorf("cost = %v, want 0.25", total.Cost)
	}
	// a free call alone is a known cost of 0, not unknown
	var free Usage
	free.Add(Usage{Cost: f64(0)})
	if free.Cost == nil || *free.Cost != 0 {
		t.Errorf("a free call: %v", free.Cost)
	}
	// the added usage is not changed and its cost is not shared
	o := Usage{Cost: f64(1)}
	var sum Usage
	sum.Add(o)
	*sum.Cost = 9
	if *o.Cost != 1 {
		t.Error("Add must copy the cost, not alias it")
	}
}
