// description_rule.js reads a job description off the currently loaded page by a RULE the person set (plan 186, step 1):
// an ordered list of parts, each with the box to take (include), what to leave out of it (exclude), an optional heading
// and an optional "cut after" text. It returns the text, a flag when the limit cut it, and a report (per part, the
// outline, what was taken out) so the screen can show that nothing real was lost.
//
// It never changes the page: every box is read from a copy. The reading of text is the one extract.js uses (code,
// styling and media are left out, blocks become lines), plus a "structured" format with headings on their own lines
// and "- " before list items. Cookie / consent / dialog boxes inside a box are always left out (a consent dialog beat
// the real description as the "largest block" on every Arbeitnow page in the spike).
//
// The payload token on the last line is replaced by extract.go's counterpart (description_rule.go) before the script
// runs. It is deliberately not written out in this comment (only the first occurrence is replaced).
(function (payload) {
  var rule = payload.rule;
  var limit = rule.limit;
  var structured = rule.format === "structured";
  var MAX_MATCHES = 50;
  var MAX_REMOVED = 30;
  var MAX_OUTLINE = 40;
  var PREVIEW = 200;

  var NO_TEXT = "script, style, noscript, template, svg, iframe, object, embed, canvas, img, picture, video, audio, source, track, link, meta";
  var BLOCK = {
    p: 1, div: 1, li: 1, ul: 1, ol: 1, dl: 1, dt: 1, dd: 1, tr: 1, table: 1, section: 1, article: 1, header: 1,
    footer: 1, nav: 1, aside: 1, main: 1, blockquote: 1, pre: 1, hr: 1, form: 1, fieldset: 1, figure: 1,
    figcaption: 1, address: 1, details: 1, summary: 1, h1: 1, h2: 1, h3: 1, h4: 1, h5: 1, h6: 1
  };
  var CELL = { td: 1, th: 1 };
  // In the structured format these get an empty line around them.
  var SPACED = { p: 1, h1: 1, h2: 1, h3: 1, h4: 1, h5: 1, h6: 1, ul: 1, ol: 1, table: 1, blockquote: 1, pre: 1 };

  function collect(node, out, pre) {
    if (node.nodeType === 3) {
      out.push(pre ? node.nodeValue : node.nodeValue.replace(/\s+/g, " "));
      return;
    }
    if (node.nodeType !== 1) return;
    var tag = node.tagName.toLowerCase();
    if (tag === "br") {
      out.push("\n");
      return;
    }
    var isBlock = BLOCK[tag] === 1;
    var isCell = CELL[tag] === 1;
    var spaced = structured && SPACED[tag] === 1;
    if (isBlock) out.push(spaced ? "\n\n" : "\n");
    if (structured && tag === "li") out.push("- ");
    if (isCell) out.push(" ");
    var inPre = pre || tag === "pre";
    for (var c = node.firstChild; c; c = c.nextSibling) collect(c, out, inPre);
    if (isBlock) out.push(spaced ? "\n\n" : "\n");
    if (isCell) out.push(" ");
  }

  function tidy(raw) {
    var lines = raw.replace(/[\u00ad\u200b\ufeff]/g, "").split("\n");
    var kept = [];
    var blank = 0;
    for (var j = 0; j < lines.length; j++) {
      var line = lines[j].replace(/[ \t\f\v\u00a0\r]+/g, " ").trim();
      if (structured && line === "-") continue; // an empty list item ("- " and nothing) is not a line
      if (line === "") {
        blank++;
        if (blank > 1) continue;
        kept.push("");
        continue;
      }
      blank = 0;
      kept.push(line);
    }
    if (structured) {
      // an empty list item leaves blank lines between two items: close them up
      kept = kept.filter(function (l, i) {
        return !(l === "" && i > 0 && i < kept.length - 1 && kept[i - 1].indexOf("- ") === 0 && kept[i + 1].indexOf("- ") === 0);
      });
    }
    return kept.join("\n").trim();
  }

  function textOf(el) {
    var out = [];
    collect(el, out, false);
    return tidy(out.join(""));
  }

  function preview(s) {
    s = s.replace(/\s+/g, " ").trim();
    return s.length > PREVIEW ? s.slice(0, PREVIEW) : s;
  }

  var removed = [];
  var removedTotal = 0;
  function noteRemoved(part, kind, selector, text) {
    removedTotal++;
    if (removed.length < MAX_REMOVED) {
      removed.push({ part: part, kind: kind, selector: selector, chars: text.length, text: preview(text) });
    }
  }

  var errors = [];
  function safeAll(root, selector, where) {
    try {
      return root.querySelectorAll(selector);
    } catch (e) {
      errors.push(where + ": the selector is not valid: " + selector);
      return [];
    }
  }

  // The nearest heading (h1-h4) written before the box, at the same level or in a parent's earlier siblings.
  function headingBefore(el) {
    var node = el;
    for (var depth = 0; depth < 4 && node; depth++) {
      var sib = node.previousElementSibling;
      for (var n = 0; sib && n < 20; n++, sib = sib.previousElementSibling) {
        if (/^h[1-4]$/i.test(sib.tagName)) return textOf(sib);
        var inner = sib.querySelectorAll ? sib.querySelectorAll("h1,h2,h3,h4") : [];
        if (inner.length) return textOf(inner[inner.length - 1]);
      }
      node = node.parentElement;
    }
    return "";
  }

  var taken = [];
  var outline = [];
  var report = [];
  var partTexts = [];

  for (var p = 0; p < rule.parts.length; p++) {
    var part = rule.parts[p];
    var info = { index: p + 1, include: part.include, matches: 0, chars: 0, overlap: false, notFound: false, cut: 0, removed: 0 };
    var matches = safeAll(document, part.include, "part " + (p + 1));
    var boxes = [];
    for (var m = 0; m < matches.length && boxes.length < MAX_MATCHES; m++) {
      var el = matches[m];
      var inside = false;
      for (var t = 0; t < taken.length; t++) {
        if (taken[t] === el || taken[t].contains(el)) {
          inside = true;
          break;
        }
        if (el.contains(taken[t])) info.overlap = true;
      }
      if (inside) {
        info.overlap = true;
        continue;
      }
      boxes.push(el);
      taken.push(el);
    }
    info.matches = boxes.length;
    if (boxes.length === 0) {
      info.notFound = true;
      report.push(info);
      continue;
    }

    var bodies = [];
    for (var b = 0; b < boxes.length; b++) {
      var clone = boxes[b].cloneNode(true);
      var drop = clone.querySelectorAll(NO_TEXT);
      for (var d = 0; d < drop.length; d++) drop[d].remove();
      var consent = safeAll(clone, payload.consentSelector, "consent");
      for (var c1 = 0; c1 < consent.length; c1++) {
        if (clone.contains(consent[c1])) {
          var ct = textOf(consent[c1]);
          if (ct) {
            noteRemoved(p + 1, "consent", "", ct);
            info.removed += ct.length;
          }
          consent[c1].remove();
        }
      }
      for (var x = 0; x < (part.exclude || []).length; x++) {
        var sel = part.exclude[x];
        var hit = safeAll(clone, sel, "part " + (p + 1) + " exclude");
        for (var h = 0; h < hit.length; h++) {
          if (!clone.contains(hit[h]) || hit[h] === clone) continue;
          var et = textOf(hit[h]);
          if (et) {
            noteRemoved(p + 1, "exclude", sel, et);
            info.removed += et.length;
          }
          hit[h].remove();
        }
      }
      var heads = clone.querySelectorAll("h1,h2,h3,h4");
      for (var hh = 0; hh < heads.length && outline.length < MAX_OUTLINE; hh++) {
        var ht = textOf(heads[hh]);
        if (ht) {
          ht = ht.replace(/\s+/g, " ");
          outline.push(ht.length > 80 ? ht.slice(0, 80) : ht);
        }
      }
      bodies.push(textOf(clone));
    }
    var text = bodies.filter(function (s) { return s !== ""; }).join("\n\n");

    if (part.heading === "auto" || part.heading === "selector") {
      var head = "";
      if (part.heading === "selector" && part.headingSelector) {
        var hs = safeAll(document, part.headingSelector, "part " + (p + 1) + " heading");
        if (hs.length) head = textOf(hs[0]);
      } else {
        head = headingBefore(boxes[0]);
      }
      var firstLine = text.split("\n")[0] || "";
      if (head && head !== firstLine) text = head + "\n\n" + text;
    }

    var cutTexts = part.cutAfter || [];
    if (cutTexts.length && text) {
      var lines = text.split("\n");
      for (var li = 0; li < lines.length; li++) {
        var low = lines[li].trim().toLowerCase();
        var hitCut = false;
        for (var ci = 0; ci < cutTexts.length; ci++) {
          if (cutTexts[ci] !== "" && low.indexOf(cutTexts[ci].toLowerCase()) === 0) {
            hitCut = true;
            break;
          }
        }
        if (hitCut) {
          var dropped = lines.slice(li).join("\n");
          noteRemoved(p + 1, "cutAfter", cutTexts[ci], dropped);
          info.cut = dropped.length;
          info.removed += dropped.length;
          text = lines.slice(0, li).join("\n").trim();
          break;
        }
      }
    }
    info.chars = text.length;
    report.push(info);
    if (text) partTexts.push(text);
  }

  var full = tidy(partTexts.join("\n\n"));
  var truncated = false;
  var fullLength = full.length;
  if (full.length > limit) {
    truncated = true;
    full = full.slice(0, limit);
    var last = full.charCodeAt(full.length - 1);
    if (last >= 0xd800 && last <= 0xdbff) full = full.slice(0, -1);
  }

  return {
    text: full,
    chars: full.length,
    fullLength: fullLength,
    truncated: truncated,
    limit: limit,
    parts: report,
    outline: outline,
    removed: removed,
    removedTotal: removedTotal,
    errors: errors
  };
})(__DESCRIPTION_PAYLOAD__)
