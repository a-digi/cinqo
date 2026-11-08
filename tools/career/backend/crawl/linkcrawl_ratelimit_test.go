package crawl

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func noRealWait(t *testing.T) *[]time.Duration {
	var waits []time.Duration
	orig := sleepRateWait
	sleepRateWait = func(_ context.Context, d time.Duration) { waits = append(waits, d) }
	t.Cleanup(func() { sleepRateWait = orig })
	return &waits
}

func limitedErr(secs int) error {
	return &linkFetchError{Code: "rate_limited", Message: "slow down", RetryAfterSeconds: secs}
}

func TestARateLimitedPageIsRetriedAfterTheAdvisedWaitThenSucceeds(t *testing.T) {
	waits := noRealWait(t)
	calls := 0
	var said []string
	v, err := fetchWithRateRetry(context.Background(), func(s string) { said = append(said, s) }, func() (int, error) {
		calls++
		if calls < 3 {
			return 0, limitedErr(45)
		}
		return 7, nil
	})
	if err != nil || v != 7 || calls != 3 {
		t.Fatalf("v=%d err=%v calls=%d", v, err, calls)
	}
	if len(*waits) != 2 || (*waits)[0] != 45*time.Second || len(said) != 2 || !strings.Contains(said[0], "try 1 of 2") {
		t.Errorf("waits=%v said=%v", *waits, said)
	}
}

func TestAPageStillLimitedAfterTwoRetriesStopsAndOtherErrorsAreNotRetried(t *testing.T) {
	noRealWait(t)
	calls := 0
	_, err := fetchWithRateRetry(context.Background(), func(string) {}, func() (int, error) { calls++; return 0, limitedErr(0) })
	if err == nil || calls != 3 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	if got := linkFetchFailureText(err); !strings.Contains(got, "limiting requests") || !strings.Contains(got, "30 s") {
		t.Errorf("text %q", got)
	}
	calls = 0
	_, err = fetchWithRateRetry(context.Background(), func(string) {}, func() (int, error) {
		calls++
		return 0, &linkFetchError{Code: "cloudflare_challenge_unresolved", Message: "x"}
	})
	if err == nil || calls != 1 {
		t.Errorf("a challenge is not retried: calls=%d", calls)
	}
}

func TestAStopEndsTheWaitWithoutAnotherTry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	orig := sleepRateWait
	sleepRateWait = func(context.Context, time.Duration) { cancel() }
	t.Cleanup(func() { sleepRateWait = orig })
	calls := 0
	_, err := fetchWithRateRetry(ctx, func(string) {}, func() (int, error) { calls++; return 0, limitedErr(10) })
	if err == nil || calls != 1 {
		t.Errorf("calls=%d err=%v", calls, err)
	}
}

func TestTheWaitIsCappedAtFiveMinutes(t *testing.T) {
	if got := rateLimitWaitOf(&linkFetchError{RetryAfterSeconds: 99999}); got != maxRateLimitWait {
		t.Errorf("%v", got)
	}
}

func TestALimitedLinkRunWaitsRetriesThenFailsWithTheLimitText(t *testing.T) {
	waits := noRealWait(t)
	id := linkCrawlLink(t)
	f := newFakeCore(t, "katana")
	f.fetchStatus = http.StatusBadGateway
	f.fetchBody = `{"code":"rate_limited","message":"The site is rate limiting this tool","retryAfterSeconds":40}`
	if code, resp := crawlNow(t, id); code != http.StatusAccepted {
		t.Fatalf("%d %s", code, resp)
	}
	status, _, errMsg, _ := waitRun(t, id)
	if status != "failed" || !strings.Contains(errMsg, "limiting requests") {
		t.Errorf("%s %q", status, errMsg)
	}
	n := 0
	for _, p := range f.paths {
		if strings.HasSuffix(p, "/fetch-links") {
			n++
		}
	}
	if n != 3 || len(*waits) != 2 || (*waits)[0] != 40*time.Second {
		t.Errorf("fetches=%d waits=%v", n, *waits)
	}
}

// plan 197, B2 follow-up: the paginated path's 429 keeps its code and is never worded as a Cloudflare block.
func TestABrowserRateLimitAnswerIsNotWordedAsACloudflareBlock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"code":"rate_limited","message":"The site is rate limiting this tool","reason":"HTTP 429","retryAfterSeconds":25}`))
	}))
	defer srv.Close()
	_, err := callBrowserProxy(context.Background(), srv.URL, "/x", []byte(`{}`), "tok")
	var fe *linkFetchError
	if !errors.As(err, &fe) || fe.Code != "rate_limited" || fe.RetryAfterSeconds != 25 {
		t.Fatalf("%v", err)
	}
	if strings.Contains(err.Error(), "Cloudflare") || !strings.HasPrefix(err.Error(), rateLimitErrorPrefix) {
		t.Errorf("wording: %q", err.Error())
	}
}
