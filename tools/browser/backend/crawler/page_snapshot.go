// page_snapshot.go (plan 247, step 1) — the page as the Career picker shows it: the live DOM of the page /fetch-links just read, made safe to
// render in a sandboxed iframe that runs no script.
//
// The page-side script (snapshotCaptureJS) only reports: the document's HTML, its style sheets (the readable ones as text) and its base URL. Everything
// that makes it safe is done HERE, in Go, on a parsed tree, because a hostile page can tamper with its own JavaScript. The rules:
//   - elements that run or load code are dropped (script, noscript, iframe, frame, object, embed, applet, base, meta, link, style, template, svg script
//     and foreignObject), every on* attribute and the attributes that navigate or submit are dropped;
//   - every address is made absolute against the page and only http(s) survives (data: only for images); a link keeps its address in data-href and
//     its href becomes "#", so a click can never leave the view;
//   - the style sheets are inlined (the unreadable cross-origin ones stay a remote <link>), @import is dropped;
//   - a Content-Security-Policy meta denies everything but styles, images and fonts.
//
// The anchors carry data-cinqo-a="n", the number the link list reports in `anchors`, so the picker can tie a selection in the view to a row of the list.
package crawler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const (
	// maxSnapshotHTML bounds the stored page; a longer one is cut at a tag boundary and reported truncated.
	maxSnapshotHTML = 8 << 20
	// maxSnapshotCSS bounds the inlined style sheets together; a sheet that does not fit stays a remote link.
	maxSnapshotCSS = 2 << 20
	// maxSnapshotCapture bounds what the page may hand back at all.
	maxSnapshotCapture = 24 << 20
	// snapshotCSP is the policy inside the shown document: styles, images and fonts, nothing else.
	snapshotCSP = "default-src 'none'; style-src 'unsafe-inline' https: http:; img-src https: http: data:; font-src https: http: data:"
)

type snapshotCtxKey struct{}

// withSnapshot asks the page read to also make a snapshot.
func withSnapshot(ctx context.Context, want bool) context.Context {
	if !want {
		return ctx
	}
	return context.WithValue(ctx, snapshotCtxKey{}, true)
}

func wantsSnapshot(ctx context.Context) bool {
	v, _ := ctx.Value(snapshotCtxKey{}).(bool)
	return v
}

// snapshotCaptureJS reports the live document. It runs after pageLinksScript(true) has numbered the anchors.
const snapshotCaptureJS = `(function () {
	var sheets = [];
	function add(s) {
		var css = null;
		try { var r = s.cssRules, out = []; for (var i = 0; i < r.length; i++) out.push(r[i].cssText); css = out.join("\n"); } catch (e) { css = null; }
		sheets.push({h: s.href || "", c: css, m: (s.media && s.media.mediaText) || ""});
	}
	for (var i = 0; i < document.styleSheets.length; i++) add(document.styleSheets[i]);
	try { (document.adoptedStyleSheets || []).forEach(add); } catch (e) {}
	return JSON.stringify({html: "<!DOCTYPE html>" + document.documentElement.outerHTML, sheets: sheets, base: document.baseURI});
})()`

type snapshotSheet struct {
	Href  string  `json:"h"`
	CSS   *string `json:"c"`
	Media string  `json:"m"`
}

type snapshotCapture struct {
	HTML   string          `json:"html"`
	Sheets []snapshotSheet `json:"sheets"`
	Base   string          `json:"base"`
}

var (
	cssURLRe    = regexp.MustCompile(`(?i)url\(\s*(?:"([^"]*)"|'([^']*)'|([^)\s]*))\s*\)`)
	cssImportRe = regexp.MustCompile(`(?is)@import\b[^;{]*;?`)
)

// absoluteWeb resolves ref against base and returns it only when the result is an http(s) address without credentials.
func absoluteWeb(base *url.URL, ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", false
	}
	u, err := url.Parse(ref)
	if err != nil {
		return "", false
	}
	if base != nil {
		u = base.ResolveReference(u)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return "", false
	}
	return u.String(), true
}

// cleanCSS makes the addresses of a style sheet absolute against its own base and drops @import and anything that is not http(s) or data:.
func cleanCSS(css string, base *url.URL) string {
	css = cssImportRe.ReplaceAllString(css, "")
	return cssURLRe.ReplaceAllStringFunc(css, func(m string) string {
		sub := cssURLRe.FindStringSubmatch(m)
		ref := sub[1] + sub[2] + sub[3]
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(ref)), "data:") {
			return m
		}
		if abs, ok := absoluteWeb(base, ref); ok {
			return `url("` + strings.NewReplacer(`"`, `%22`, `\`, `%5C`, "\n", "").Replace(abs) + `")`
		}
		return `url("")`
	})
}

// droppedElements are matched by lower-cased name, because the parser keeps the case of svg elements (foreignObject) and gives them no atom.
var droppedElements = map[string]bool{
	"script": true, "noscript": true, "iframe": true, "frame": true, "frameset": true, "object": true, "embed": true,
	"applet": true, "base": true, "meta": true, "link": true, "style": true, "template": true, "foreignobject": true,
}

var droppedAttributes = map[string]bool{
	"srcdoc": true, "action": true, "formaction": true, "ping": true, "target": true, "integrity": true, "nonce": true, "http-equiv": true,
	"data-href": true, "loading": true, "referrerpolicy": true, "crossorigin": true,
}

func attrIndex(n *html.Node, key string) int {
	for i, a := range n.Attr {
		if a.Namespace == "" && a.Key == key {
			return i
		}
	}
	return -1
}

func attrValue(n *html.Node, key string) string {
	if i := attrIndex(n, key); i >= 0 {
		return n.Attr[i].Val
	}
	return ""
}

func setAttr(n *html.Node, key, val string) {
	if i := attrIndex(n, key); i >= 0 {
		n.Attr[i].Val = val
		return
	}
	n.Attr = append(n.Attr, html.Attribute{Key: key, Val: val})
}

func removeAttr(n *html.Node, key string) {
	if i := attrIndex(n, key); i >= 0 {
		n.Attr = append(n.Attr[:i], n.Attr[i+1:]...)
	}
}

// cleanSrcset resolves every candidate of a srcset and drops the ones that are not http(s).
func cleanSrcset(base *url.URL, v string) string {
	var out []string
	for _, cand := range strings.Split(v, ",") {
		f := strings.Fields(cand)
		if len(f) == 0 {
			continue
		}
		if abs, ok := absoluteWeb(base, f[0]); ok {
			out = append(out, strings.Join(append([]string{abs}, f[1:]...), " "))
		}
	}
	return strings.Join(out, ", ")
}

// cleanElement filters the attributes of one element in place.
func cleanElement(n *html.Node, base *url.URL) {
	isAnchor := n.DataAtom == atom.A || n.DataAtom == atom.Area
	kept := n.Attr[:0]
	for _, a := range n.Attr {
		key := strings.ToLower(a.Key)
		switch {
		case strings.HasPrefix(key, "on"), droppedAttributes[key]:
			continue
		case strings.HasPrefix(key, "data-cinqo-"):
			if !(key == "data-cinqo-a" && n.DataAtom == atom.A) {
				continue
			}
		}
		kept = append(kept, a)
	}
	n.Attr = kept

	if isAnchor {
		if i := attrIndex(n, "href"); i >= 0 {
			if abs, ok := absoluteWeb(base, n.Attr[i].Val); ok {
				n.Attr[i].Val = "#"
				setAttr(n, "data-href", abs)
			} else {
				removeAttr(n, "href")
			}
		}
	}
	for i := 0; i < len(n.Attr); i++ {
		a := &n.Attr[i]
		key := strings.ToLower(a.Key)
		switch {
		case key == "href" && isAnchor:
		case key == "href" && (a.Namespace != "" || n.Data == "use"):
			// an svg reference (<use href>, xlink:href): only a fragment of the same document stays
			if !strings.HasPrefix(strings.TrimSpace(a.Val), "#") {
				n.Attr = append(n.Attr[:i], n.Attr[i+1:]...)
				i--
			}
		case key == "href":
			// href on anything else (an svg <image href>) must be a web address or it goes
			if abs, ok := absoluteWeb(base, a.Val); ok {
				a.Val = abs
			} else {
				n.Attr = append(n.Attr[:i], n.Attr[i+1:]...)
				i--
			}
		case key == "src" || key == "poster" || key == "data-src" || key == "data-original":
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(a.Val)), "data:image/") && (n.DataAtom == atom.Img || n.DataAtom == atom.Source) {
				continue
			}
			if abs, ok := absoluteWeb(base, a.Val); ok {
				a.Val = abs
			} else {
				n.Attr = append(n.Attr[:i], n.Attr[i+1:]...)
				i--
			}
		case key == "srcset" || key == "data-srcset":
			if v := cleanSrcset(base, a.Val); v != "" {
				a.Val = v
			} else {
				n.Attr = append(n.Attr[:i], n.Attr[i+1:]...)
				i--
			}
		case key == "style":
			a.Val = cleanCSS(a.Val, base)
		}
	}
	if n.DataAtom == atom.Img || n.DataAtom == atom.Source {
		if attrValue(n, "src") == "" {
			if v := firstNonEmpty(attrValue(n, "data-src"), attrValue(n, "data-original")); v != "" {
				setAttr(n, "src", v)
			}
		}
	}
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

// sanitizeTree removes every dropped element below n and cleans the rest.
func sanitizeTree(n *html.Node, base *url.URL) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		if c.Type == html.ElementNode {
			if droppedElements[strings.ToLower(c.Data)] {
				n.RemoveChild(c)
				c = next
				continue
			}
			cleanElement(c, base)
		}
		if c.Type == html.CommentNode {
			n.RemoveChild(c)
			c = next
			continue
		}
		sanitizeTree(c, base)
		c = next
	}
}

func findElement(n *html.Node, a atom.Atom) *html.Node {
	if n.Type == html.ElementNode && n.DataAtom == a {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if f := findElement(c, a); f != nil {
			return f
		}
	}
	return nil
}

func elementNode(a atom.Atom, attrs ...html.Attribute) *html.Node {
	return &html.Node{Type: html.ElementNode, DataAtom: a, Data: a.String(), Attr: attrs}
}

// pageBase is the address the page's relative references resolve against: the document's own base URL, else the final URL.
func pageBase(capture snapshotCapture, finalURL string) *url.URL {
	for _, s := range []string{capture.Base, finalURL} {
		if u, err := url.Parse(s); err == nil && u.Host != "" {
			return u
		}
	}
	return nil
}

// buildSnapshotHTML turns the captured document into the safe copy. truncated: the page or its styles did not fit the bounds.
func buildSnapshotHTML(capture snapshotCapture, finalURL string) (out string, truncated bool, err error) {
	base := pageBase(capture, finalURL)
	doc, err := html.Parse(strings.NewReader(capture.HTML))
	if err != nil {
		return "", false, fmt.Errorf("the page could not be parsed: %w", err)
	}
	sanitizeTree(doc, base)
	head := findElement(doc, atom.Head)
	if head == nil {
		return "", false, fmt.Errorf("the page has no head")
	}

	var inlined strings.Builder
	var remote []string
	for _, s := range capture.Sheets {
		var sheetBase *url.URL = base
		if s.Href != "" {
			if u, e := url.Parse(s.Href); e == nil {
				sheetBase = u
			}
		}
		if s.CSS != nil && inlined.Len()+len(*s.CSS) <= maxSnapshotCSS {
			css := cleanCSS(*s.CSS, sheetBase)
			if m := strings.TrimSpace(s.Media); m != "" && m != "all" {
				css = "@media " + m + " {\n" + css + "\n}"
			}
			inlined.WriteString(css)
			inlined.WriteString("\n")
			continue
		}
		if s.CSS != nil {
			truncated = true
		}
		if abs, ok := absoluteWeb(nil, s.Href); ok {
			remote = append(remote, abs)
		}
	}

	var first *html.Node = head.FirstChild
	add := func(n *html.Node) { head.InsertBefore(n, first) }
	add(elementNode(atom.Meta, html.Attribute{Key: "charset", Val: "utf-8"}))
	add(elementNode(atom.Meta, html.Attribute{Key: "http-equiv", Val: "Content-Security-Policy"}, html.Attribute{Key: "content", Val: snapshotCSP}))
	for _, r := range remote {
		add(elementNode(atom.Link, html.Attribute{Key: "rel", Val: "stylesheet"}, html.Attribute{Key: "href", Val: r}))
	}
	if inlined.Len() > 0 {
		st := elementNode(atom.Style, html.Attribute{Key: "data-cinqo-sheets", Val: "1"})
		st.AppendChild(&html.Node{Type: html.RawNode, Data: strings.ReplaceAll(inlined.String(), "</", `<\/`)})
		add(st)
	}

	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return "", false, fmt.Errorf("the page could not be written: %w", err)
	}
	out = buf.String()
	if len(out) > maxSnapshotHTML {
		cut := strings.LastIndex(out[:maxSnapshotHTML], ">")
		if cut < 0 {
			cut = maxSnapshotHTML
		}
		out = out[:cut+1]
		truncated = true
	}
	return out, truncated, nil
}

// parseSnapshotCapture decodes what snapshotCaptureJS returned.
func parseSnapshotCapture(raw string) (snapshotCapture, error) {
	var c snapshotCapture
	if len(raw) > maxSnapshotCapture {
		return c, fmt.Errorf("the page is too large to show (%d MB)", len(raw)>>20)
	}
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return c, fmt.Errorf("the page copy could not be decoded: %w", err)
	}
	return c, nil
}
