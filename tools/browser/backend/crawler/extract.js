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
  function truncate(value) {
    if (typeof value === "string" && maxValueLength && value.length > maxValueLength) {
      return value.slice(0, maxValueLength);
    }
    return value;
  }

  function readValue(el, attribute) {
    if (!attribute || attribute === "text") {
      return truncate((el.textContent || "").trim());
    }
    return truncate(el.getAttribute(attribute));
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
          values.push(readValue(matches[j], f.attribute));
        }
        obj[f.label] = values;
      } else if (matches.length === 0) {
        notFoundSeen[f.label] = true;
      } else {
        obj[f.label] = readValue(matches[0], f.attribute);
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
    return { items: items, notFound: Object.keys(notFoundSeen) };
  }

  var results = extractFieldsFrom(document, notFoundSeen);
  return { results: results, notFound: Object.keys(notFoundSeen) };
})(__EXTRACT_PAYLOAD__)
