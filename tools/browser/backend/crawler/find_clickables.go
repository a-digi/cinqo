// find_clickables.go (Namako plan 17, step 4) — POST /find-clickables: the controls and the frames of a page that a person (or the AI that helps them) may want
// to press so that a player starts. It is READ-ONLY: it opens the page the way /fetch-frames does, looks, and never clicks, types or follows anything. Each item
// comes with a selector that matches exactly ONE element of the page (so the Namako side can build a page recipe from it without inventing a selector) and with
// where it is on the page, so a frame can be pressed by its position.
//
// It also LISTS (never presses) the layers that may be in the way of the player: a cookie banner, an age gate, a skip bar, a close button, or whatever covers the player.
//
// What counts: visible buttons, links, tabs and elements that look pressable (a role, an onclick handler, a pointer cursor) whose words (text, title, aria-label,
// id, class) talk about playing or choosing a server, plus every visible frame of a useful size (a player is a frame). The page's own words are DATA: they are cut
// short, never evaluated, and the Namako side validates anything built from them.
package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	maxLayers             = 8
	maxClickables         = 40
	maxClickableFrames    = 10
	defaultClickablesWait = 1500
	maxClickablesWait     = 8000
	clickableTextMax      = 60
	clickableSelectorMax  = 200
)

// Clickable is one control or frame of a page.
type Clickable struct {
	ID       string   `json:"id"`   // c1, c2 … for controls, f1, f2 … for frames
	Kind     string   `json:"kind"` // "control" or "frame"
	Tag      string   `json:"tag"`
	Text     string   `json:"text,omitempty"`
	Label    string   `json:"label,omitempty"` // the title or aria-label
	Selector string   `json:"selector"`
	Host     string   `json:"host,omitempty"`   // a frame's site
	Href     string   `json:"href,omitempty"`   // a link's address, cut short
	Leaves   bool     `json:"leaves,omitempty"` // pressing it probably leaves the page (a link to another site)
	X        int      `json:"x"`
	Y        int      `json:"y"`
	Width    int      `json:"width"`
	Height   int      `json:"height"`
	Why      []string `json:"why,omitempty"` // in words: what made it a candidate
}

// Layer is something that may be in the way of the player: a cookie banner, an age gate, a "skip" bar, a close button, or the layer that covers the player. It is
// only LISTED: the Browser never presses it. A person decides (per page) whether it may be pressed before the player is tried.
type Layer struct {
	ID           string `json:"id"`   // l1, l2 …
	Kind         string `json:"kind"` // cookie, age, skip, close or cover
	Tag          string `json:"tag"`
	Text         string `json:"text,omitempty"`
	Label        string `json:"label,omitempty"`
	Selector     string `json:"selector"`
	Leaves       bool   `json:"leaves,omitempty"`
	CoversPlayer bool   `json:"coversPlayer,omitempty"` // it sits on top of the player (or of a play control)
	X            int    `json:"x"`
	Y            int    `json:"y"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
}

// ClickablesResult is the answer of /find-clickables.
type ClickablesResult struct {
	FinalURL  string      `json:"finalUrl"`
	Items     []Clickable `json:"items"`
	Truncated bool        `json:"truncated"`
	// Layers are what may be in the way (see Layer); never part of Items.
	Layers []Layer `json:"layers"`
}

type findClickablesRequest struct {
	URL       string `json:"url"`
	WaitMs    int    `json:"waitMs,omitempty"`
	RequestID string `json:"requestId,omitempty"`
}

// clickablesScript lists the candidates. It only reads: no event is dispatched and nothing is focused or scrolled.
const clickablesScript = `(function(){
  var WORDS = /play|watch|stream|server|source|player|episode|quality|cinema|continue|accept|agree|start|embed|tab|mirror|link/i;
  var out = [], controls = [], frames = [];
  var origin = location.origin, host = location.hostname;
  function clean(s){ return (s||"").replace(/\s+/g," ").trim(); }
  function visible(e){
    var r = e.getBoundingClientRect(), cs = getComputedStyle(e);
    return r.width > 0 && r.height > 0 && cs.visibility !== "hidden" && cs.display !== "none" && parseFloat(cs.opacity||"1") > 0.01 && r.bottom > -5000 && r.top < 20000;
  }
  function uniq(s){ try { return document.querySelectorAll(s).length === 1; } catch (x) { return false; } }
  function esc(s){ return (window.CSS && CSS.escape) ? CSS.escape(s) : s.replace(/[^a-zA-Z0-9_-]/g, "\\$&"); }
  function classes(e){
    var c = (e.getAttribute("class")||"").split(/\s+/).filter(function(x){ return x && x.length < 30 && !/\d{3,}/.test(x) && !/^(active|current|selected|hover|focus|open|show|hide|visible|hidden)$/i.test(x); });
    return c.slice(0, 2);
  }
  function part(e){ var t = e.tagName.toLowerCase(); var c = classes(e); return t + c.map(function(x){ return "." + esc(x); }).join(""); }
  function selector(e){
    if (e.id && !/\d{4,}/.test(e.id) && uniq("#" + esc(e.id))) return "#" + esc(e.id);
    var p = part(e);
    if (uniq(p)) return p;
    var cur = e, chain = [p];
    for (var i = 0; i < 4 && cur.parentElement && cur.parentElement !== document.body; i++) {
      cur = cur.parentElement;
      if (cur.id && !/\d{4,}/.test(cur.id) && uniq("#" + esc(cur.id) + " " + chain.join(" > "))) return "#" + esc(cur.id) + " " + chain.join(" > ");
      chain.unshift(part(cur));
      var s = chain.join(" > ");
      if (uniq(s)) return s;
    }
    // last resort: the position among the siblings, from the nearest ancestor with an id
    var path = [], n = e;
    for (var j = 0; j < 8 && n && n.nodeType === 1 && n !== document.documentElement; j++) {
      var idx = 1, sib = n;
      while ((sib = sib.previousElementSibling)) if (sib.tagName === n.tagName) idx++;
      path.unshift(n.tagName.toLowerCase() + ":nth-of-type(" + idx + ")");
      n = n.parentElement;
      var cand = "html > " + path.join(" > ");
      if (uniq(cand)) return cand;
    }
    return "";
  }
  function box(e){ var r = e.getBoundingClientRect(); return { x: Math.round(r.left + window.scrollX), y: Math.round(r.top + window.scrollY), width: Math.round(r.width), height: Math.round(r.height) }; }
  var seen = new Set();
  Array.prototype.forEach.call(document.querySelectorAll("iframe"), function(f){
    if (!visible(f)) return;
    var b = box(f);
    if (b.width < 120 || b.height < 80) return;
    var src = f.getAttribute("src") || "", h = "";
    try { h = src ? new URL(src, location.href).hostname : ""; } catch (x) {}
    var sel = selector(f);
    if (!sel) return;
    seen.add(f);
    frames.push({ kind: "frame", tag: "iframe", text: clean(f.getAttribute("title") || f.getAttribute("name") || ""), selector: sel, host: h || host, x: b.x, y: b.y, width: b.width, height: b.height, why: [h && h !== host ? "a player frame from another site" : "a frame of this site"] });
  });
  var nodes = document.querySelectorAll("button, a, [role=button], [role=tab], [onclick], input[type=button], input[type=submit], li, div, span, label");
  Array.prototype.forEach.call(nodes, function(e, order){
    if (seen.has(e) || !visible(e)) return;
    var tag = e.tagName.toLowerCase();
    var text = clean(e.innerText || e.value || "");
    if (text.length > 120) text = text.slice(0, 120);
    var label = clean(e.getAttribute("aria-label") || e.getAttribute("title") || "");
    var idc = (e.id || "") + " " + (e.getAttribute("class") || "");
    var role = e.getAttribute("role") || "";
    var pressable = tag === "button" || tag === "a" || role === "button" || role === "tab" || e.hasAttribute("onclick") || tag === "input" || getComputedStyle(e).cursor === "pointer";
    if (!pressable) return;
    var reasons = [];
    if (WORDS.test(text)) reasons.push("its text talks about playing or choosing a source");
    if (WORDS.test(label)) reasons.push("its label talks about playing or choosing a source");
    if (WORDS.test(idc)) reasons.push("its id or class talks about a player or a server");
    if (reasons.length === 0) return;
    var r = e.getBoundingClientRect();
    if (r.width * r.height > 900000) return;  // a whole-page wrapper is not a button
    if (text.length > 60 && !WORDS.test(label)) return;  // a block of text is not a button
    var sel = selector(e);
    if (!sel || sel.length > 200) return;
    var href = tag === "a" ? (e.getAttribute("href") || "") : "", leaves = false, hrefShort = "";
    if (href && !/^(#|javascript:)/i.test(href)) {
      try { var u = new URL(href, location.href); hrefShort = (u.hostname + u.pathname).slice(0, 80); leaves = u.origin !== origin || u.pathname !== location.pathname; } catch (x) {}
    }
    var score = reasons.length + (tag === "button" || role === "button" || role === "tab" ? 1 : 0) - (leaves ? 2 : 0);
    var b = box(e);
    controls.push({ order: order, score: score, item: { kind: "control", tag: tag, text: text, label: label, selector: sel, href: hrefShort, leaves: leaves, x: b.x, y: b.y, width: b.width, height: b.height, why: reasons } });
  });
  controls.sort(function(a, b){ return b.score - a.score || a.order - b.order; });

  // Layers: things that may be in the way of the player. Read only (elementFromPoint, no scroll, no event).
  var layers = [], layerSeen = new Set();
  var CLOSE_TEXT = /^(x|\u00d7|\u2715|\u2716|\u2573|close|dismiss|no thanks|not now|maybe later)$/i;
  var CLOSE_WORDS = /close|dismiss|btn-x|modal-x/i;
  var SKIP_TEXT = /^(skip( ad| ads)?|close ad|continue to (video|site|player))$/i;
  var AGE_TEXT = /(over|above|older than)\s*18|\b18\s*\+|i am 18|enter (the )?site|i'm 18/i;
  var COOKIE_TEXT = /^(accept( all| cookies)?|agree|i agree|allow( all)?|got it|ok|okay|consent)$/i;
  var COOKIE_CTX = /cookie|consent|gdpr|privacy|cmp|onetrust/i;
  function ancestorsText(e){ var t = "", c = e; for (var i = 0; i < 5 && c && c !== document.body; i++) { t += " " + (c.id||"") + " " + (c.getAttribute("class")||""); c = c.parentElement; } return t; }
  function pressableEl(e){ var tag = e.tagName.toLowerCase(); var role = e.getAttribute("role") || ""; return tag === "button" || tag === "a" || role === "button" || e.hasAttribute("onclick") || tag === "input" || getComputedStyle(e).cursor === "pointer"; }
  function kindOfControl(e){
    var text = clean(e.innerText || e.value || ""), label = clean(e.getAttribute("aria-label") || e.getAttribute("title") || ""), idc = (e.id||"") + " " + (e.getAttribute("class")||"");
    var r = e.getBoundingClientRect();
    if (AGE_TEXT.test(text) || AGE_TEXT.test(label)) return "age";
    if (SKIP_TEXT.test(text) || SKIP_TEXT.test(label)) return "skip";
    if (COOKIE_TEXT.test(text) && COOKIE_CTX.test(ancestorsText(e))) return "cookie";
    if ((CLOSE_TEXT.test(text) || CLOSE_TEXT.test(label) || CLOSE_WORDS.test(idc)) && r.width <= 160 && r.height <= 160) return "close";
    return "";
  }
  function addLayer(e, kind, covers){
    if (layers.length >= 12 || layerSeen.has(e)) return;
    var sel = selector(e);
    if (!sel || sel.length > 200) return;
    layerSeen.add(e);
    var tag = e.tagName.toLowerCase(), href = tag === "a" ? (e.getAttribute("href") || "") : "", leaves = false;
    if (href && !/^(#|javascript:)/i.test(href)) { try { var u = new URL(href, location.href); leaves = u.origin !== origin || u.pathname !== location.pathname; } catch (x) {} }
    var b = box(e);
    layers.push({ kind: kind, tag: tag, text: clean(e.innerText || e.value || "").slice(0, 120), label: clean(e.getAttribute("aria-label") || e.getAttribute("title") || ""), selector: sel, leaves: leaves, coversPlayer: !!covers, x: b.x, y: b.y, width: b.width, height: b.height });
  }
  // 1. what sits on top of a player frame or a play control
  var targets = Array.prototype.slice.call(document.querySelectorAll("iframe")).filter(function(f){ return seen.has(f); });
  controls.slice(0, 3).forEach(function(c){ try { var el = document.querySelector(c.item.selector); if (el) targets.push(el); } catch (x) {} });
  targets.forEach(function(t){
    var r = t.getBoundingClientRect(), cx = r.left + r.width / 2, cy = r.top + r.height / 2;
    if (cx < 0 || cy < 0 || cx > window.innerWidth || cy > window.innerHeight) return;
    var top = document.elementFromPoint(cx, cy);
    if (!top || top === t || t.contains(top) || top === document.body || top === document.documentElement) return;
    var root = top;
    while (root.parentElement && root.parentElement !== document.body && root.parentElement !== document.documentElement) {
      var pr = root.parentElement.getBoundingClientRect();
      if (pr.left > cx || pr.right < cx || pr.top > cy || pr.bottom < cy || pr.width * pr.height > window.innerWidth * window.innerHeight * 1.5) break;
      root = root.parentElement;
    }
    var inner = Array.prototype.slice.call(root.querySelectorAll("button, a, [role=button], [onclick], span, div, i")).filter(function(e){ return visible(e) && pressableEl(e) && kindOfControl(e) !== ""; });
    if (inner.length > 0) addLayer(inner[0], kindOfControl(inner[0]) === "cookie" || kindOfControl(inner[0]) === "age" || kindOfControl(inner[0]) === "skip" ? kindOfControl(inner[0]) : "close", true);
    else addLayer(root, "cover", true);
  });
  // 2. controls that are a cookie banner, an age gate, a skip bar or a close button anywhere on the page
  Array.prototype.forEach.call(document.querySelectorAll("button, a, [role=button], [onclick], input[type=button], span, div, i"), function(e){
    if (!visible(e) || !pressableEl(e)) return;
    var k = kindOfControl(e);
    if (k) addLayer(e, k, false);
  });
  var rank = { cover: 0, close: 1, age: 2, cookie: 3, skip: 4 };
  layers.sort(function(a, b){ return (b.coversPlayer ? 1 : 0) - (a.coversPlayer ? 1 : 0) || rank[a.kind] - rank[b.kind]; });
  return { frames: frames, controls: controls.map(function(c){ return c.item; }), layers: layers };
})()`

type clickablesRaw struct {
	Frames   []Clickable `json:"frames"`
	Controls []Clickable `json:"controls"`
	Layers   []Layer     `json:"layers"`
}

var layerKinds = map[string]bool{"cookie": true, "age": true, "skip": true, "close": true, "cover": true}

// arrangeLayers numbers the layers (l1..), cuts the words, drops what has no usable selector or a kind that is not one of the five, and keeps at most maxLayers.
func arrangeLayers(in []Layer) []Layer {
	out := []Layer{}
	for _, l := range in {
		if len(out) >= maxLayers {
			break
		}
		if !layerKinds[l.Kind] || strings.TrimSpace(l.Selector) == "" || len(l.Selector) > clickableSelectorMax || strings.ContainsAny(l.Selector, "\x00\r\n") {
			continue
		}
		l.ID = fmt.Sprintf("l%d", len(out)+1)
		l.Text, l.Label = cutText(l.Text, clickableTextMax), cutText(l.Label, clickableTextMax)
		out = append(out, l)
	}
	return out
}

// cutText keeps a page's words short and one line.
func cutText(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return s
}

// arrangeClickables numbers the items (frames f1.., controls c1..), cuts the words, drops what has no usable selector, and caps the list.
func arrangeClickables(raw clickablesRaw) (items []Clickable, truncated bool) {
	items = []Clickable{}
	add := func(c Clickable, prefix string, n *int) {
		if strings.TrimSpace(c.Selector) == "" || len(c.Selector) > clickableSelectorMax || strings.ContainsAny(c.Selector, "\x00\r\n") {
			return
		}
		*n++
		c.ID = fmt.Sprintf("%s%d", prefix, *n)
		c.Text, c.Label = cutText(c.Text, clickableTextMax), cutText(c.Label, clickableTextMax)
		c.Href = cutText(c.Href, 80)
		items = append(items, c)
	}
	nf, nc := 0, 0
	for _, f := range raw.Frames {
		if nf >= maxClickableFrames {
			truncated = true
			break
		}
		f.Kind = "frame"
		add(f, "f", &nf)
	}
	for _, c := range raw.Controls {
		if len(items) >= maxClickables {
			truncated = true
			break
		}
		c.Kind = "control"
		add(c, "c", &nc)
	}
	return items, truncated
}

// FindClickablesHandler handles POST /find-clickables.
func FindClickablesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req findClickablesRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBodySize))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil || strings.TrimSpace(req.URL) == "" {
		http.Error(w, "url is required (and only url, waitMs and requestId are accepted)", http.StatusBadRequest)
		return
	}
	if req.WaitMs < 0 {
		http.Error(w, "waitMs must not be negative", http.StatusBadRequest)
		return
	}
	if err := validateCrawlURL(req.URL); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := runEvidence(r.Context(), req.RequestID, func(ctx context.Context) (any, error) {
		return findClickables(ctx, req)
	})
	writeEvidenceResult(w, req.RequestID, res, err, "find clickables")
}

// findClickables opens the page, lets it settle, and reads what can be pressed. Nothing is pressed.
func findClickables(ctx context.Context, req findClickablesRequest) (ClickablesResult, error) {
	ep, err := openEvidencePage(ctx, req.URL, req.RequestID)
	if err != nil {
		return ClickablesResult{}, err
	}
	defer ep.Close()
	if err := sleepCtx(ctx, time.Duration(clampInt(req.WaitMs, defaultClickablesWait, maxClickablesWait))*time.Millisecond); err != nil {
		return ClickablesResult{}, err
	}
	rawJSON, err := rodEvalExpression(ep.P, clickablesScript)
	if err != nil {
		return ClickablesResult{}, rodCtxOr(ctx, fmt.Errorf("could not read the page's controls: %w", err))
	}
	var raw clickablesRaw
	if err := json.Unmarshal(rawJSON, &raw); err != nil {
		return ClickablesResult{}, fmt.Errorf("could not read the page's controls: %w", err)
	}
	final := req.URL
	if info, ierr := ep.P.Info(); ierr == nil && info != nil {
		final = info.URL
	}
	items, truncated := arrangeClickables(raw)
	return ClickablesResult{FinalURL: final, Items: items, Truncated: truncated, Layers: arrangeLayers(raw.Layers)}, nil
}
