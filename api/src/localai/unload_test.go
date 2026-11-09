package localai

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/a-digi/cinqo/src/localai/entity"
)

// --- the HTTP handler (plan 225, step 2) ---

func TestUnloadStopsTheProgramAndAnswersStopped(t *testing.T) {
	db := openMainDB(t)
	var got []bool
	rec := callWith("LocalAiUnload", fakeRuntime{unloaded: &got}, &fakeInstalls{}, db, `{}`)
	if rec.Code != http.StatusOK || len(got) != 1 || got[0] {
		t.Fatalf("status %d force %v body %s", rec.Code, got, rec.Body.String())
	}
	var out struct {
		Message struct {
			State string `json:"state"`
		} `json:"message"`
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Message.State != "stopped" || !out.Success {
		t.Errorf("%s %v", rec.Body.String(), err)
	}
}

func TestUnloadWithoutABodyIsTheSameAsAnEmptyOne(t *testing.T) {
	db := openMainDB(t)
	var got []bool
	for _, body := range []string{"", "{}", `{"force":false}`} {
		got = nil
		rec := callWith("LocalAiUnload", fakeRuntime{unloaded: &got}, &fakeInstalls{}, db, body)
		if rec.Code != http.StatusOK || len(got) != 1 || got[0] {
			t.Errorf("body %q: status %d force %v", body, rec.Code, got)
		}
	}
}

func TestUnloadRefusesWithA409WhileAConversationUsesTheModelAndForceOverridesIt(t *testing.T) {
	db := openMainDB(t)
	var got []bool
	rt := fakeRuntime{unloaded: &got, inUse: true}
	rec := callWith("LocalAiUnload", rt, &fakeInstalls{}, db, `{}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "a conversation is using the model right now") {
		t.Errorf("status %d body %s", rec.Code, rec.Body.String())
	}
	rec = callWith("LocalAiUnload", rt, &fakeInstalls{}, db, `{"force":true}`)
	if rec.Code != http.StatusOK || len(got) != 2 || got[0] || !got[1] {
		t.Errorf("status %d forces %v", rec.Code, got)
	}
}

func TestUnloadRefusesABodyThatIsNotJSON(t *testing.T) {
	db := openMainDB(t)
	var got []bool
	for _, body := range []string{"{broken", "force", `{"force":"yes"}`, "[1]"} {
		rec := callWith("LocalAiUnload", fakeRuntime{unloaded: &got}, &fakeInstalls{}, db, body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %q: status %d, want 400", body, rec.Code)
		}
	}
	if len(got) != 0 {
		t.Errorf("a refused request must not stop anything: %v", got)
	}
}

func TestUnloadWithoutAManagedEngineStopsNothingAndIsStillFine(t *testing.T) {
	old := managed
	t.Cleanup(func() { managed = old })
	managed = nil
	if got := (managedRuntime{}).Unload(false); got != entity.UnloadStopped {
		t.Errorf("%v", got)
	}
}

// --- the supervisor ---

func TestUnloadAnIdleProgramStopsItAndTheNextRequestStartsItAgain(t *testing.T) {
	var starts atomic.Int32
	s := newSup(t, &starts)
	spec := testSpec(t)
	_, _, release, err := s.ensure(context.Background(), spec, 100) // an idle time of 6 s
	if err != nil {
		t.Fatal(err)
	}
	release()
	if v := s.view(); v.state != EngineReady || v.unloadsIn <= 0 {
		t.Fatalf("idle with a countdown: %+v", v)
	}
	if got := s.unload(false); got != entity.UnloadStopped {
		t.Fatalf("%v", got)
	}
	if v := s.view(); v.state != EngineStopped || v.unloadsIn != 0 || v.busy {
		t.Errorf("after the unload: %+v (the countdown must be cleared)", v)
	}
	_, _, rel2, err := s.ensure(context.Background(), spec, 0)
	if err != nil {
		t.Fatalf("the next request must start the program again: %v", err)
	}
	rel2()
	if starts.Load() != 2 {
		t.Errorf("%d starts, want 2", starts.Load())
	}
}

func TestUnloadAProgramThatARequestIsUsingIsRefusedUnlessForced(t *testing.T) {
	var starts atomic.Int32
	s := newSup(t, &starts)
	_, _, release, err := s.ensure(context.Background(), testSpec(t), 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.unload(false); got != entity.UnloadInUse {
		t.Fatalf("a request is under way: %v", got)
	}
	if st := s.Status(); st.State != EngineReady {
		t.Errorf("a refused unload must stop nothing: %+v", st)
	}
	if got := s.unload(true); got != entity.UnloadStopped {
		t.Fatalf("forced: %v", got)
	}
	if st := s.Status(); st.State != EngineStopped {
		t.Errorf("%+v", st)
	}
	release() // the request ends later: releasing must not make the count negative or start anything
	if v := s.view(); v.state != EngineStopped || v.busy {
		t.Errorf("%+v", v)
	}
}

func TestUnloadAProgramThatIsStillLoadingEndsTheLoadWithoutAsking(t *testing.T) {
	var starts atomic.Int32
	s := newSup(t, &starts, "FAKE_LLAMA_DELAY_MS=600")
	errs := make(chan error, 1)
	go func() {
		_, _, release, err := s.ensure(context.Background(), testSpec(t), 0)
		if err == nil {
			release()
		}
		errs <- err
	}()
	waitUntil(t, "the load to begin", func() bool { return s.view().state == EngineStarting })
	if got := s.unload(false); got != entity.UnloadStopped {
		t.Fatalf("a load is not \"in use\": %v", got)
	}
	select {
	case err := <-errs:
		if err == nil {
			t.Error("the waiting request must get an error, not a program that was unloaded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the load must end")
	}
	if st := s.Status(); st.State != EngineStopped {
		t.Errorf("%+v", st)
	}
}

func TestUnloadWhenNothingRunsIsHarmlessAndRepeatable(t *testing.T) {
	var starts atomic.Int32
	s := newSup(t, &starts)
	for i := 0; i < 3; i++ {
		if got := s.unload(false); got != entity.UnloadStopped {
			t.Errorf("call %d: %v", i, got)
		}
	}
	if starts.Load() != 0 {
		t.Error("unloading must never start the program")
	}
}

func TestUnloadClearsAFailedState(t *testing.T) {
	var starts atomic.Int32
	s := newSup(t, &starts, "FAKE_LLAMA_CRASH_MS=150")
	_, _, rel, err := s.ensure(context.Background(), testSpec(t), 0)
	if err != nil {
		t.Fatal(err)
	}
	rel()
	waitUntil(t, "the crash", func() bool { return s.view().state == EngineFailed })
	if got := s.unload(false); got != entity.UnloadStopped {
		t.Fatalf("%v", got)
	}
	if st := s.Status(); st.State != EngineStopped || st.Error != "" {
		t.Errorf("an unload resets a failed state: %+v", st)
	}
}

func TestStopStillStopsAProgramARequestIsUsing(t *testing.T) {
	var starts atomic.Int32
	s := newSup(t, &starts)
	_, _, release, err := s.ensure(context.Background(), testSpec(t), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	s.Stop() // the application shutting down never asks
	if st := s.Status(); st.State != EngineStopped {
		t.Errorf("%+v", st)
	}
}

// The force flag travels the whole way: the runtime the handler uses, the managed engine and the supervisor.
func TestTheForceFlagReachesTheSupervisorThroughTheManagedEngine(t *testing.T) {
	var starts atomic.Int32
	s := newSup(t, &starts)
	old := managed
	t.Cleanup(func() { managed = old })
	managed = &managedEngine{sup: s}
	_, _, release, err := s.ensure(context.Background(), testSpec(t), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if got := (managedRuntime{}).Unload(false); got != entity.UnloadInUse {
		t.Fatalf("not forced: %v", got)
	}
	if st := s.Status(); st.State != EngineReady {
		t.Fatalf("a refused unload stops nothing: %+v", st)
	}
	if got := (managedRuntime{}).Unload(true); got != entity.UnloadStopped {
		t.Fatalf("forced: %v", got)
	}
	if st := s.Status(); st.State != EngineStopped {
		t.Errorf("%+v", st)
	}
}
