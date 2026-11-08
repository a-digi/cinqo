// diagnose.go (step 96) answers "what does the tool actually see when it opens
// this URL?": the main document's status and the headers that matter, whether
// Cloudflare issued a clearance cookie, what the page probe found, and the
// verdict the tool would act on. For a person debugging a site that "is not
// recognized": the answer is in these fields, not in a guess. Read-only; it
// navigates a worker tab like find_repeating_items and reports, nothing more.
// See plan/ai/tools/browser/step-96-cloudflare-outcomes-and-kept-clearance.md.
package crawler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/chromedp/chromedp"

	"browser-tool-backend/shared"
)

// diagnoseReport is what GET /diagnose-url returns.
type diagnoseReport struct {
	URL        string `json:"url"`
	FinalURL   string `json:"finalUrl"`
	Title      string `json:"title"`
	HTTPStatus int64  `json:"httpStatus"`
	Wire       struct {
		Server           string `json:"server,omitempty"`
		CFRay            string `json:"cfRay,omitempty"`
		CFMitigated      string `json:"cfMitigated,omitempty"`
		RetryAfter       string `json:"retryAfter,omitempty"`
		ClearanceIssued  int    `json:"clearanceCookiesIssued"`
		MainDocumentSeen bool   `json:"mainDocumentSeen"`
	} `json:"wire"`
	Page struct {
		CloudflareChallengePage bool   `json:"cloudflareChallengePage"`
		CloudflareBlockPage     bool   `json:"cloudflareBlockPage"`
		CloudflareErrorCode     string `json:"cloudflareErrorCode,omitempty"`
		OtherVendor             string `json:"otherVendor,omitempty"`
		TurnstileWidgetVisible  bool   `json:"turnstileWidgetVisible"`
		VisibleTextLength       int    `json:"visibleTextLength"`
		ReadyState              string `json:"readyState"`
	} `json:"page"`
	// Verdict is what the tool would do: "none" (an ordinary page),
	// "cloudflare_challenge" (the tracker waits for it to clear), or one of the
	// outcome kinds.
	Verdict string `json:"verdict"`
	Reason  string `json:"reason,omitempty"`
	Advice  string `json:"advice,omitempty"`
}

// diagnosePage reads the evidence from a tab that has just navigated.
func diagnosePage(ctx context.Context, signals *challengeSignals) (diagnoseReport, error) {
	var rep diagnoseReport
	tracker, err := newChallengeTracker(ctx, signals, nil)
	if err != nil {
		return rep, err
	}
	st, err := tracker.stateWithRetry(ctx)
	if err != nil {
		return rep, err
	}
	if err := chromedp.Run(ctx, chromedp.Title(&rep.Title), chromedp.Location(&rep.FinalURL)); err != nil {
		return rep, err
	}
	w, p := st.wire, st.probe
	rep.HTTPStatus = w.status
	rep.Wire.Server, rep.Wire.CFRay, rep.Wire.CFMitigated, rep.Wire.RetryAfter = w.server, w.cfRay, w.mitigated, w.retryAfter
	rep.Wire.ClearanceIssued, rep.Wire.MainDocumentSeen = w.clearanceIssued, w.docSeen
	rep.Page.CloudflareChallengePage, rep.Page.CloudflareBlockPage = p.ChallengePage, p.BlockPage
	rep.Page.CloudflareErrorCode, rep.Page.OtherVendor = p.ErrorCode, p.Vendor
	rep.Page.TurnstileWidgetVisible, rep.Page.VisibleTextLength, rep.Page.ReadyState = p.WidgetVisible, p.BodyLen, p.ReadyState

	rep.Verdict = "none"
	switch {
	case st.blocked:
		rep.Verdict, rep.Reason, rep.Advice = outcomeBlocked, st.reason, adviceFor(outcomeBlocked, 0, "")
	case st.final != nil:
		rep.Verdict, rep.Reason, rep.Advice = st.final.Outcome, st.reason, st.final.Advice
	case st.challenge:
		rep.Verdict, rep.Reason = "cloudflare_challenge", st.reason
	}
	return rep, nil
}

// DiagnoseURLHandler handles GET /diagnose-url?url=… — one headless worker
// attempt inside the AI call budget, never headed.
func DiagnoseURLHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	target := r.URL.Query().Get("url")
	if target == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}
	// The same SSRF guard every other URL-taking route applies.
	if err := validateCrawlURL(target); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	rep, err := performDiagnose(r.Context(), target)
	if err != nil {
		var cfErr *crawlError
		if errors.As(err, &cfErr) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(cfErr)
			return
		}
		http.Error(w, fmt.Sprintf("diagnose failed: %v", err), http.StatusBadGateway)
		return
	}
	rep.URL = target
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rep)
}

func performDiagnose(reqCtx context.Context, target string) (diagnoseReport, error) {
	deadline := aiDeadline("", time.Now())
	if capTimeout(paginatedCrawlTimeout, deadline) < minWorkerAttempt {
		return diagnoseReport{}, errors.New("time budget exhausted before a browser tab could be used")
	}
	tabCtx, release, err := shared.AcquireWorkerTab(reqCtx)
	if err != nil {
		return diagnoseReport{}, err
	}
	defer release()

	ctx, cancel := context.WithTimeout(tabCtx, capTimeout(paginatedCrawlTimeout, deadline))
	defer cancel()
	linkRequestCancel(reqCtx, ctx, cancel, tabCtx)

	signals := watchChallengeSignals(ctx)
	seedStoredClearance(ctx, target) // step 96
	if err := navigateWithHangGuard(ctx, release, chromedp.Navigate(target), chromedp.Sleep(SettleDelay)); err != nil {
		return diagnoseReport{}, classifyCancellation(err, reqCtx, tabCtx)
	}
	rep, err := diagnosePage(ctx, signals)
	return rep, classifyCancellation(err, reqCtx, tabCtx)
}
