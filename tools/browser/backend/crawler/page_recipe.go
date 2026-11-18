// page_recipe.go (Namako plan 17, step 1) — the "page recipe" of POST /capture-network: a few presses that make a player start before the media requests
// are collected. Some pirate pages request nothing until a person presses a server tab, a cover or the player itself; the fixed play script cannot do that.
//
// The vocabulary is closed and tiny, and it is data, never code:
//   - click       a real mouse click at the centre of the first element the CSS selector matches in the TOP document;
//   - clickFrame  the same for an <iframe> element: the click lands inside the frame, also when the frame is another site, because the mouse event is sent
//     at page coordinates through the DevTools input, not through the frame's DOM (no script access to a foreign frame is needed or used);
//   - wait        a pause of 1..5000 ms.
//
// Nothing is typed, submitted, evaluated or navigated by a step. A selector goes to querySelector only (inside a JSON-encoded string, never concatenated),
// and an element that is missing, hidden or of the wrong kind makes THAT step "skipped" with a note, never an error that hides the rest. After every step the
// popups the page opened are closed, and when the click took the top page somewhere else (an ad redirect) the run stops, goes back to the original address
// once, and says so.
package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

// The limits of a recipe.
const (
	maxRecipeSteps    = 6
	maxStepSelector   = 200
	maxStepWaitMs     = 5000
	maxRecipeWaitMs   = 10000
	stepFindTries     = 3
	stepFindEvery     = 500 * time.Millisecond
	settleAfterClick  = 400 * time.Millisecond
	stepDoClick       = "click"
	stepDoClickFrame  = "clickFrame"
	stepDoWait        = "wait"
	stepDone          = "done"
	stepSkipped       = "skipped"
	stepStopped       = "stopped"
	stepNotRun        = "not run"
	outcomeLeftTheTop = "the click took the page somewhere else; the run stopped and went back"
)

// RecipeStep is one press or pause of a recipe.
type RecipeStep struct {
	Do       string `json:"do"`
	Selector string `json:"selector,omitempty"`
	Ms       int    `json:"ms,omitempty"`
}

// StepOutcome says what became of one step.
type StepOutcome struct {
	Index  int    `json:"index"`
	Do     string `json:"do"`
	Status string `json:"status"` // done, skipped, stopped, not run
	Note   string `json:"note,omitempty"`
	// Popups is how many tabs this step made the page open (an ad, usually); they were closed at once. Zero is left out.
	Popups int `json:"popups,omitempty"`
}

// validateRecipe checks the whole recipe before anything runs.
func validateRecipe(steps []RecipeStep) error {
	if len(steps) > maxRecipeSteps {
		return fmt.Errorf("a recipe has at most %d steps", maxRecipeSteps)
	}
	total := 0
	for i, s := range steps {
		switch s.Do {
		case stepDoClick, stepDoClickFrame:
			if strings.TrimSpace(s.Selector) == "" {
				return fmt.Errorf("step %d: a %s needs a selector", i+1, s.Do)
			}
			if len(s.Selector) > maxStepSelector {
				return fmt.Errorf("step %d: the selector is longer than %d characters", i+1, maxStepSelector)
			}
			if strings.ContainsAny(s.Selector, "\x00\r\n") {
				return fmt.Errorf("step %d: the selector has a control character", i+1)
			}
			if s.Ms != 0 {
				return fmt.Errorf("step %d: only a wait has a duration", i+1)
			}
		case stepDoWait:
			if s.Ms < 1 || s.Ms > maxStepWaitMs {
				return fmt.Errorf("step %d: a wait is between 1 and %d ms", i+1, maxStepWaitMs)
			}
			if s.Selector != "" {
				return fmt.Errorf("step %d: a wait has no selector", i+1)
			}
			total += s.Ms
		default:
			return fmt.Errorf(`step %d: "do" must be "click", "clickFrame" or "wait"`, i+1)
		}
	}
	if total > maxRecipeWaitMs {
		return fmt.Errorf("the waits of a recipe add up to at most %d ms", maxRecipeWaitMs)
	}
	return nil
}

// targetProbe finds the element and reports where to press. The selector is passed as a JSON string literal: it is never part of the script text.
func targetProbe(selector string) string {
	lit, _ := json.Marshal(selector)
	return `(function(sel){
  var e;
  try { e = document.querySelector(sel); } catch (x) { return {error: "the selector is not valid"}; }
  if (!e) return {found: false};
  try { e.scrollIntoView({block: "center", inline: "center"}); } catch (x) {}
  var r = e.getBoundingClientRect(), cs = getComputedStyle(e);
  var visible = cs.visibility !== "hidden" && cs.display !== "none" && r.width > 0 && r.height > 0;
  var x = r.left + r.width / 2, y = r.top + r.height / 2;
  var top = visible ? document.elementFromPoint(x, y) : null;
  var onTop = !!top && (top === e || e.contains(top) || top.contains(e));
  var inView = x >= 0 && y >= 0 && x <= window.innerWidth && y <= window.innerHeight;
  return {found: true, tag: e.tagName, visible: visible, x: x, y: y, onTop: onTop, inView: inView};
})(` + string(lit) + `)`
}

type probeResult struct {
	Error   string  `json:"error"`
	Found   bool    `json:"found"`
	Tag     string  `json:"tag"`
	Visible bool    `json:"visible"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	OnTop   bool    `json:"onTop"`
	InView  bool    `json:"inView"`
}

// probe looks for the element, a few times (a press may have just made it appear).
func probe(ctx context.Context, ep *evidencePage, selector string) (probeResult, error) {
	var res probeResult
	for try := 0; try < stepFindTries; try++ {
		raw, err := rodEvalExpression(ep.P, targetProbe(selector))
		if err != nil {
			return res, err
		}
		res = probeResult{}
		if err := json.Unmarshal(raw, &res); err != nil {
			return res, err
		}
		if res.Error != "" || (res.Found && res.Visible) {
			return res, nil
		}
		if try < stepFindTries-1 {
			if err := sleepCtx(ctx, stepFindEvery); err != nil {
				return res, err
			}
		}
	}
	return res, nil
}

// topAddress is the address of the top page now, without its fragment.
func topAddress(ep *evidencePage) string {
	info, err := ep.P.Info()
	if err != nil || info == nil {
		return ""
	}
	u := info.URL
	if i := strings.Index(u, "#"); i >= 0 {
		u = u[:i]
	}
	return u
}

// runRecipe runs the steps in order on an open page and says what happened to each. It stops early only when the caller cancels or a click took the top page
// somewhere else. popups is how many tabs the page opened and were closed on the way.
func runRecipe(ctx context.Context, ep *evidencePage, steps []RecipeStep) (out []StepOutcome, popups int, err error) {
	out = make([]StepOutcome, 0, len(steps))
	start := topAddress(ep)
	for i, s := range steps {
		o := StepOutcome{Index: i + 1, Do: s.Do}
		switch s.Do {
		case stepDoWait:
			if err := sleepCtx(ctx, time.Duration(s.Ms)*time.Millisecond); err != nil {
				return out, popups, err
			}
			o.Status = stepDone
		default:
			pr, perr := probe(ctx, ep, s.Selector)
			switch {
			case ctx.Err() != nil:
				return out, popups, ctx.Err()
			case perr != nil:
				o.Status, o.Note = stepSkipped, "the page could not be asked about this element"
			case pr.Error != "":
				o.Status, o.Note = stepSkipped, pr.Error
			case !pr.Found:
				o.Status, o.Note = stepSkipped, "nothing on the page matches this selector"
			case !pr.Visible:
				o.Status, o.Note = stepSkipped, "the element is not visible"
			case s.Do == stepDoClickFrame && pr.Tag != "IFRAME":
				o.Status, o.Note = stepSkipped, "this is not a frame (it is a "+strings.ToLower(pr.Tag)+")"
			case !pr.InView:
				o.Status, o.Note = stepSkipped, "the element is outside the window"
			default:
				if err := ep.P.Mouse.MoveTo(proto.Point{X: pr.X, Y: pr.Y}); err != nil {
					o.Status, o.Note = stepSkipped, "the pointer could not be moved there"
					break
				}
				if err := ep.P.Mouse.Click(proto.InputMouseButtonLeft, 1); err != nil {
					o.Status, o.Note = stepSkipped, "the click could not be sent"
					break
				}
				o.Status = stepDone
				if !pr.OnTop {
					o.Note = "another element covered it: that one was pressed"
				}
				if err := sleepCtx(ctx, settleAfterClick); err != nil {
					return append(out, o), popups, err
				}
			}
		}
		o.Popups = closeOwnPopups(ep)
		popups += o.Popups
		out = append(out, o)
		if now := topAddress(ep); start != "" && now != "" && now != start {
			out[len(out)-1].Status, out[len(out)-1].Note = stepStopped, outcomeLeftTheTop
			for j := i + 1; j < len(steps); j++ {
				out = append(out, StepOutcome{Index: j + 1, Do: steps[j].Do, Status: stepNotRun})
			}
			// back to where we were, once; a failure here only ends the recipe (the page is read as it is)
			_ = ep.P.Navigate(start)
			_ = ep.P.WaitLoad()
			return out, popups, nil
		}
	}
	return out, popups, nil
}
