package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/conversation"
	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
)

func threeMessages() []messageResponse {
	return []messageResponse{
		{Role: "user", Content: "old", Failed: true, TurnRunID: "t-old"},
		{Role: "user", Content: "q", TurnRunID: "t-ok"},
		{Role: "assistant", Content: "a", TurnRunID: "t-ok"},
		{Role: "user", Content: "last", Failed: true, TurnRunID: "t-last"},
	}
}

func TestOnlyTheFailedMessageOfTheLastRunIsMarkedRetryable(t *testing.T) {
	msgs := threeMessages()
	attachRetryInfo(msgs, &conversation_entity.TurnRun{ID: "t-last", Status: "failed", RetryCount: 2}, true)
	for i, m := range msgs {
		want := i == 3
		if m.Retryable != want {
			t.Errorf("message %d retryable = %v, want %v", i, m.Retryable, want)
		}
	}
	if msgs[3].RetryCount == nil || *msgs[3].RetryCount != 2 {
		t.Errorf("RetryCount = %v, want 2", msgs[3].RetryCount)
	}
	if msgs[0].RetryCount != nil {
		t.Error("an older failed turn carries no retry count")
	}
}

func TestNothingIsRetryableWithoutACheckpointOrForANonFailedRun(t *testing.T) {
	for name, c := range map[string]struct {
		run *conversation_entity.TurnRun
		has bool
	}{
		"no checkpoint":     {&conversation_entity.TurnRun{ID: "t-last", Status: "failed"}, false},
		"completed":         {&conversation_entity.TurnRun{ID: "t-last", Status: "completed"}, true},
		"cancelled":         {&conversation_entity.TurnRun{ID: "t-last", Status: "cancelled"}, true},
		"no run":            {nil, true},
		"run not in thread": {&conversation_entity.TurnRun{ID: "elsewhere", Status: "failed"}, true},
	} {
		msgs := threeMessages()
		attachRetryInfo(msgs, c.run, c.has)
		for i, m := range msgs {
			if m.Retryable || m.RetryCount != nil {
				t.Errorf("%s: message %d was marked", name, i)
			}
		}
	}
}

func TestAFirstRetryableFailureHasNoRetryCountInTheJSON(t *testing.T) {
	msgs := threeMessages()
	attachRetryInfo(msgs, &conversation_entity.TurnRun{ID: "t-last", Status: "failed"}, true)
	raw, _ := json.Marshal(msgs[3])
	var got map[string]any
	_ = json.Unmarshal(raw, &got)
	if got["retryable"] != true {
		t.Errorf("retryable missing: %s", raw)
	}
	if _, has := got["retryCount"]; has {
		t.Errorf("a count of 0 is left out like the other counts: %s", raw)
	}
}

func TestTheRetryErrorsMapToTheRightStatusAndTheConfirmationCarriesItsCode(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{&conversation.RetryConfirmationRequiredError{RetryCount: 3}, http.StatusConflict},
		{conversation.ErrTurnRunNotFound, http.StatusNotFound},
		{conversation.ErrTurnNotRetryable, http.StatusConflict},
		{conversation.ErrTurnAlreadyRunning, http.StatusConflict},
		{errors.New("boom with internals"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		writeRetryError(rec, c.err)
		if rec.Code != c.status {
			t.Errorf("%v: status %d, want %d", c.err, rec.Code, c.status)
		}
		if body := rec.Body.String(); c.status == http.StatusInternalServerError && (len(body) == 0 || strings.Contains(body, "internals")) {
			t.Errorf("a 500 must not leak the error: %s", body)
		}
	}
	rec := httptest.NewRecorder()
	writeRetryError(rec, &conversation.RetryConfirmationRequiredError{RetryCount: 3})
	var body retryConfirmationBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || !body.Error || body.Code != "retry_confirmation_required" || body.RetryCount != 3 {
		t.Errorf("confirmation body: %+v %v", body, err)
	}
	other := httptest.NewRecorder()
	writeRetryError(other, conversation.ErrTurnNotRetryable)
	if strings.Contains(other.Body.String(), "retry_confirmation_required") {
		t.Error("only the confirmation answer carries the code")
	}
}
