// extract.js reads a caller-supplied list of named fields
// (label/selector/attribute/multiple) off the currently loaded page
// and returns either { results, notFound } (flat mode) or { items,
// notFound } (grouped mode, when payload.container is set) —
// evaluated as a single expression via chromedp.Evaluate, its return
// value JSON-serialized back into Go, same mechanic as
// loginheuristic.js. See
// plan/ai/tools/browser/step-09-yaml-instructed-extraction.md and
// step-18-grouped-container-extraction.md.
//
// The final line's own placeholder token is replaced with a JSON
// object (`{"container": "...", "fields": [...], "maxItems": N}`) by
// extract.go before this script is ever run — a plain JSON literal is
// valid JS syntax directly, no string-escaping involved. (Deliberately
// not spelled out literally in this comment: extract.go's
// strings.Replace call only replaces the first match, and a second
// literal copy up here would be replaced instead of the real one
// below — caught live, see step 9's own "Implemented and verified"
// section.)
(function (payload) {
  var maxValueLength = payload.maxValueLength;
  var maxItems = payload.maxItems;
  var fields = payload.fields || [];

  // Bounds one matched value's own length — maxItems only ever bounded
  // the *count* of matches for a multiple field, not one value's own
  // size, so a selector accidentally matching a huge container element
  // (e.g. a whole <body> instead of a small title) could otherwise
  // still return one enormous string.
  // A value that was cut is no longer cut silently: its label is reported in "truncated".
  var truncatedSeen = {};
  function truncate(value, label) {
    if (typeof value === "string" && maxValueLength && value.length > maxValueLength) {
      if (label) truncatedSeen[label] = true;
      return value.slice(0, maxValueLength);
    }
    return value;
  }

  // The elements that carry no text of their own: code, styling, fallbacks, media. el.textContent counts the content of
  // <style> and <script>, and of <noscript> (a browser keeps a noscript's children as plain text when scripts run, so a
  // "<style>…</style><img …>" fallback comes back as literal markup), so a field read through it fills up with CSS and tags
  // (step 147).
  var NO_TEXT = "script, style, noscript, template, svg, iframe, object, embed, canvas, img, picture, video, audio, source, track, link, meta";

  // The elements that start a new line, and the ones that are separated by a space (table cells). The same lists as the Career
  // tool's own text cleaner (backend/textclean), which tidies what is stored.
  var BLOCK = {
    p: 1, div: 1, li: 1, ul: 1, ol: 1, dl: 1, dt: 1, dd: 1, tr: 1, table: 1, section: 1, article: 1, header: 1,
    footer: 1, nav: 1, aside: 1, main: 1, blockquote: 1, pre: 1, hr: 1, form: 1, fieldset: 1, figure: 1,
    figcaption: 1, address: 1, details: 1, summary: 1, h1: 1, h2: 1, h3: 1, h4: 1, h5: 1, h6: 1
  };
  var CELL = { td: 1, th: 1 };

  function collect(node, out, pre) {
    if (node.nodeType === 3) {
      out.push(pre ? node.nodeValue : node.nodeValue.replace(/\s+/g, " "));
      return;
    }
    if (node.nodeType !== 1) {
      return;
    }
    var tag = node.tagName.toLowerCase();
    if (tag === "br") {
      out.push("\n");
      return;
    }
    var isBlock = BLOCK[tag] === 1;
    var isCell = CELL[tag] === 1;
    if (isBlock) out.push("\n");
    if (isCell) out.push(" ");
    var inPre = pre || tag === "pre";
    for (var c = node.firstChild; c; c = c.nextSibling) {
      collect(c, out, inPre);
    }
    if (isBlock) out.push("\n");
    if (isCell) out.push(" ");
  }

  // The readable text of el: what is left after the no-text elements are taken out of a copy (the page itself is not touched),
  // with a line break around blocks and at <br>, spaces collapsed, blank-line runs squeezed to one, invisible characters
  // (soft hyphen, zero-width space) dropped.
  function readableText(el) {
    if (el.matches(NO_TEXT)) {
      return "";
    }
    var clone = el.cloneNode(true);
    var drop = clone.querySelectorAll(NO_TEXT);
    for (var i = 0; i < drop.length; i++) {
      drop[i].remove();
    }
    var out = [];
    collect(clone, out, false);
    var lines = out.join("").replace(/[\u00ad\u200b\ufeff]/g, "").split("\n");
    var kept = [];
    var blank = 0;
    for (var j = 0; j < lines.length; j++) {
      var line = lines[j].replace(/[ \t\f\v\u00a0\r]+/g, " ").trim();
      if (line === "") {
        blank++;
        if (blank > 1) continue;
      } else {
        blank = 0;
      }
      kept.push(line);
    }
    return kept.join("\n").trim();
  }

  function readValue(el, attribute, label) {
    if (!attribute || attribute === "text") {
      return truncate(readableText(el), label);
    }
    return truncate(el.getAttribute(attribute), label);
  }

  // Reads every field relative to `scope` (either `document`, flat
  // mode, or one container element, grouped mode) — the one piece of
  // logic both modes below share. notFoundSeen is a plain object used
  // as a set (keys only) so a label missing from several containers in
  // grouped mode still reports only once, not once per occurrence.
  function extractFieldsFrom(scope, notFoundSeen) {
    var obj = {};
    for (var i = 0; i < fields.length; i++) {
      var f = fields[i];
      var matches;
      try {
        matches = scope.querySelectorAll(f.selector);
      } catch (e) {
        // An invalid selector is a caller error, not a crash — reported
        // back the same way a selector that simply matched nothing is.
        notFoundSeen[f.label] = true;
        continue;
      }

      if (f.multiple) {
        var values = [];
        for (var j = 0; j < matches.length && j < maxItems; j++) {
          values.push(readValue(matches[j], f.attribute, f.label));
        }
        obj[f.label] = values;
      } else if (matches.length === 0) {
        notFoundSeen[f.label] = true;
      } else {
        obj[f.label] = readValue(matches[0], f.attribute, f.label);
      }
    }
    return obj;
  }

  var notFoundSeen = {};

  if (payload.container) {
    var containers;
    try {
      containers = document.querySelectorAll(payload.container);
    } catch (e) {
      return { items: [], notFound: ["(container) " + payload.container] };
    }

    var items = [];
    for (var c = 0; c < containers.length && c < maxItems; c++) {
      items.push(extractFieldsFrom(containers[c], notFoundSeen));
    }
    return { items: items, notFound: Object.keys(notFoundSeen), truncated: Object.keys(truncatedSeen) };
  }

  var results = extractFieldsFrom(document, notFoundSeen);
  return { results: results, notFound: Object.keys(notFoundSeen), truncated: Object.keys(truncatedSeen) };
})(__EXTRACT_PAYLOAD__)
