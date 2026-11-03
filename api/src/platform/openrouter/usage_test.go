package openrouter

import (
	"testing"
)

const okChoice = `"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]`

func usageOf(t *testing.T, usageJSON string) (prompt, completion, total, cacheRead, cacheWrite, reasoning int, cost *float64) {
	t.Helper()
	body := `{` + okChoice
	if usageJSON != "" {
		body += `,"usage":` + usageJSON
	}
	body += `}`
	res, err := callWith(t, 200, body)
	if err != nil {
		t.Fatalf("%v", err)
	}
	u := res.Usage
	return u.PromptTokens, u.CompletionTokens, u.TotalTokens, u.CacheReadTokens, u.CacheCreationTokens, u.ReasoningTokens, u.Cost
}

func TestTheFullUsageOfAnOpenRouterResponseIsRead(t *testing.T) {
	p, c, tot, cr, cw, r, cost := usageOf(t, `{"prompt_tokens":12345,"completion_tokens":900,"total_tokens":13245,"cost":0.0123,
		"prompt_tokens_details":{"cached_tokens":8000,"cache_write_tokens":300},"completion_tokens_details":{"reasoning_tokens":600}}`)
	if p != 12345 || c != 900 || tot != 13245 || cr != 8000 || cw != 300 || r != 600 {
		t.Errorf("counts = %d %d %d %d %d %d", p, c, tot, cr, cw, r)
	}
	if cost == nil || *cost != 0.0123 {
		t.Errorf("cost = %v", cost)
	}
}

func TestOnlyTheThreeMainCountsLeaveTheRestZeroAndTheCostUnknown(t *testing.T) {
	p, c, tot, cr, cw, r, cost := usageOf(t, `{"prompt_tokens":34752,"completion_tokens":107,"total_tokens":34859}`)
	if p != 34752 || c != 107 || tot != 34859 || cr != 0 || cw != 0 || r != 0 || cost != nil {
		t.Errorf("%d %d %d %d %d %d %v", p, c, tot, cr, cw, r, cost)
	}
	// no usage object at all: the call still works, everything is zero or unknown
	p, c, tot, cr, cw, r, cost = usageOf(t, "")
	if p != 0 || c != 0 || tot != 0 || cr != 0 || cw != 0 || r != 0 || cost != nil {
		t.Errorf("without usage: %d %d %d %d %d %d %v", p, c, tot, cr, cw, r, cost)
	}
}

func TestAFreeModelHasACostOfZeroNotAnUnknownOne(t *testing.T) {
	_, _, _, _, _, _, cost := usageOf(t, `{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"cost":0}`)
	if cost == nil || *cost != 0 {
		t.Errorf("a free call: %v", cost)
	}
}

func TestCacheWriteAndReasoningAreReportedApartFromTheCachedTokens(t *testing.T) {
	_, _, _, cr, cw, r, _ := usageOf(t, `{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150,"prompt_tokens_details":{"cached_tokens":70}}`)
	if cr != 70 || cw != 0 || r != 0 {
		t.Errorf("only cached tokens: %d %d %d", cr, cw, r)
	}
	_, _, _, cr, cw, r, _ = usageOf(t, `{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150,"prompt_tokens_details":{"cache_write_tokens":40},"completion_tokens_details":{"reasoning_tokens":30}}`)
	if cr != 0 || cw != 40 || r != 30 {
		t.Errorf("only a cache write and reasoning: %d %d %d", cr, cw, r)
	}
}

// A number we only record must never fail a good completion: odd shapes read as "not reported".
func TestOddUsageValuesNeverFailTheCall(t *testing.T) {
	for name, usage := range map[string]string{
		"null details":      `{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6,"prompt_tokens_details":null,"completion_tokens_details":null,"cost":null}`,
		"details as text":   `{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6,"prompt_tokens_details":"none","completion_tokens_details":"none"}`,
		"details as list":   `{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6,"prompt_tokens_details":[1],"completion_tokens_details":[2]}`,
		"text numbers":      `{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6,"cost":"0.5","prompt_tokens_details":{"cached_tokens":"3"}}`,
		"negative and junk": `{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6,"cost":-1,"prompt_tokens_details":{"cached_tokens":-4,"cache_write_tokens":"many"},"completion_tokens_details":{"reasoning_tokens":true}}`,
	} {
		p, c, tot, cr, cw, r, cost := usageOf(t, usage)
		if p != 5 || c != 1 || tot != 6 {
			t.Errorf("%s: the main counts changed: %d %d %d", name, p, c, tot)
		}
		switch name {
		case "text numbers":
			if cr != 3 || cost == nil || *cost != 0.5 {
				t.Errorf("%s: %d %v", name, cr, cost)
			}
		default:
			if cr != 0 || cw != 0 || r != 0 || cost != nil {
				t.Errorf("%s: %d %d %d %v", name, cr, cw, r, cost)
			}
		}
	}
}

// The usage of a failed request is never reported: the error path is unchanged.
func TestAFailedResponseStillFailsAndCarriesNoUsage(t *testing.T) {
	res, err := callWith(t, 200, `{"choices":[{"message":{"content":""},"finish_reason":"error"}],"usage":{"prompt_tokens":34752,"completion_tokens":107,"cost":0.5}}`)
	if err == nil || res != nil {
		t.Fatalf("res=%v err=%v", res, err)
	}
}
