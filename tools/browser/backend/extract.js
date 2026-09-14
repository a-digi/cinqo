// extract.js reads a caller-supplied list of named fields
// (label/selector/attribute/multiple) off the currently loaded page
// and returns { results, notFound } — evaluated as a single
// expression via chromedp.Evaluate, its return value JSON-serialized
// back into Go, same mechanic as loginheuristic.js. See
// plan/ai/tools/browser/step-09-yaml-instructed-extraction.md.
//
// The final line's own placeholder token is replaced with a JSON
// object (`{"fields": [...], "maxItems": N}`) by extract.go before
// this script is ever run — a plain JSON literal is valid JS syntax
// directly, no string-escaping involved. (Deliberately not spelled
// out literally in this comment: extract.go's strings.Replace call
// only replaces the first match, and a second literal copy up here
// would be replaced instead of the real one below — caught live, see
// this step's own "Implemented and verified" section.)
(function (payload) {
  var maxValueLength = payload.maxValueLength;

  // Bounds one matched value's own length — maxItems (below) only
  // ever bounded the *count* of matches for a multiple field, not one
  // value's own size, so a selector accidentally matching a huge
  // container element (e.g. a whole <body> instead of a small title)
  // could otherwise still return one enormous string.
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

  var results = {};
  var notFound = [];
  var fields = payload.fields || [];
  var maxItems = payload.maxItems;

  for (var i = 0; i < fields.length; i++) {
    var f = fields[i];
    var matches;
    try {
      matches = document.querySelectorAll(f.selector);
    } catch (e) {
      // An invalid selector is a caller error, not a crash — reported
      // back the same way a selector that simply matched nothing is.
      notFound.push(f.label);
      continue;
    }

    if (f.multiple) {
      var values = [];
      for (var j = 0; j < matches.length && j < maxItems; j++) {
        values.push(readValue(matches[j], f.attribute));
      }
      results[f.label] = values;
    } else if (matches.length === 0) {
      notFound.push(f.label);
    } else {
      results[f.label] = readValue(matches[0], f.attribute);
    }
  }

  return { results: results, notFound: notFound };
})(__EXTRACT_PAYLOAD__)
