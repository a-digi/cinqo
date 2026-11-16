package crawler

import (
	"context"
	"errors"
	"testing"
)

func TestLoadWaitIsFatal(t *testing.T) {
	chain := errors.New("{-32000 Object reference chain is too long }")
	if loadWaitIsFatal(context.Background(), chain) {
		t.Fatal("a page whose load wait cannot be serialised is read as it is")
	}
	if !loadWaitIsFatal(context.Background(), errors.New("target closed")) {
		t.Fatal("any other failure still ends the visit")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !loadWaitIsFatal(ctx, chain) {
		t.Fatal("the caller's cancellation always ends the visit")
	}
}
