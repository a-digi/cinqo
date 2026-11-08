// detail_candidates.js (plan 186, step 2) — read-only analysis of ONE loaded job page: where could the title, the company,
// the location, the posted date and the description be? It returns raw candidates with features (the ranking lives in the
// Career tool, in Go), the structured data the page states about the job (JobPosting JSON-LD, meta tags), and a flat list of
// the page's text elements so a person can pick something the heuristics missed.
//
// It never changes the page, clicks nothing and requests nothing. Everything is bounded (counts, text lengths, the number of
// elements looked at) because the page is untrusted; the Go side bounds it again. The script's value is a JSON string.
(function () {
  var MAX_CANDIDATES = 8;
  var MAX_ELEMENTS = 300;
  var MAX_SCAN = 20000;
  var MAX_PREVIEW = 200;
  var MAX_LD_BLOCKS = 20;
  var MAX_LD_BYTES = 400000;
  var truncated = false;

  var SKIP = /^(script|style|noscript|template|svg|path|meta|link|html|head|iframe|object|embed|canvas|img|picture|video|audio|source|track)$/i;
  var norm = function (s) { return (s || "").replace(/\s+/g, " ").trim(); };

  // The text of an element without code, styling and media.
  function textOf(e) {
    var out = "";
    var stack = [e];
    var guard = 0;
    while (stack.length && guard++ < 5000) {
      var n = stack.pop();
      if (n.nodeType === 3) { out += n.nodeValue + " "; continue; }
      if (n.nodeType !== 1 || SKIP.test(n.tagName)) continue;
      for (var c = n.lastChild; c; c = c.previousSibling) stack.push(c);
    }
    return norm(out);
  }
  function preview(s) { s = norm(s); return s.length > MAX_PREVIEW ? s.slice(0, MAX_PREVIEW) : s; }
  function attrText(e) {
    return [e.getAttribute("itemprop"), e.getAttribute("class"), e.id, e.getAttribute("data-at"), e.getAttribute("data-testid"),
      e.getAttribute("aria-label"), e.getAttribute("data-qa")].filter(Boolean).join(" ");
  }
  function esc(s) { return window.CSS && CSS.escape ? CSS.escape(s) : s.replace(/[^a-zA-Z0-9_-]/g, "\\$&"); }

  // A selector for el that is verified on this page (it matches exactly el when "unique"). Class tokens that look generated
  // (digits, css-/sc-/jsx- prefixes) are skipped: they change between visits.
  function selectorOf(el) {
    if (el.id) {
      try { if (document.querySelectorAll("#" + esc(el.id)).length === 1) return { s: "#" + esc(el.id), unique: true }; } catch (e) {}
    }
    var parts = [];
    var e = el;
    for (var d = 0; e && e.nodeType === 1 && e !== document.body && d < 6; d++) {
      var p = e.tagName.toLowerCase();
      var cls = [];
      for (var i = 0; i < e.classList.length && cls.length < 2; i++) {
        var c = e.classList[i];
        if (!/^\d|\d{3,}|^css-|^sc-|^jsx-|^_|^svelte-|^Mui/.test(c)) cls.push(c);
      }
      if (cls.length) p += "." + cls.map(esc).join(".");
      parts.unshift(p);
      var s = parts.join(" > ");
      try { if (document.querySelectorAll(s).length === 1 && document.querySelector(s) === el) return { s: s, unique: true }; } catch (x) {}
      e = e.parentElement;
    }
    return { s: parts.join(" > "), unique: false };
  }
  function matchesOf(sel) { try { return document.querySelectorAll(sel).length; } catch (e) { return 0; } }

  var CONSENT = '[role="dialog"],[role="alertdialog"],[aria-modal="true"],dialog,[class*="cookie" i],[class*="consent" i],[id*="cookie" i],[id*="consent" i]';
  function furnitureOf(e) {
    try { if (e.closest(CONSENT)) return "consent"; } catch (x) {}
    var a = e.closest ? e.closest("nav,header,footer,aside") : null;
    return a ? a.tagName.toLowerCase() : "";
  }
  var main = document.querySelector("main,[role='main'],article") || document.body;
  function inMain(e) { return main.contains(e); }

  var all = [];
  (function collect() {
    var list = document.body ? document.body.getElementsByTagName("*") : [];
    for (var i = 0; i < list.length; i++) {
      if (i >= MAX_SCAN) { truncated = true; break; }
      if (!SKIP.test(list[i].tagName)) all.push(list[i]);
    }
  })();
  var order = new Map();
  all.forEach(function (e, i) { order.set(e, i); });

  function candidate(e, extra) {
    var s = selectorOf(e);
    var attribute = (extra && extra.attribute) || "";
    var raw = attribute ? e.getAttribute(attribute) || "" : textOf(e);
    var c = {
      selector: s.s,
      unique: s.unique,
      matches: matchesOf(s.s),
      attribute: attribute,
      value: preview(raw),
      chars: norm(raw).length,
      tag: e.tagName.toLowerCase(),
      reason: (extra && extra.reason) || "",
      inMain: inMain(e),
      placedIn: furnitureOf(e),
      order: order.has(e) ? order.get(e) : -1,
      source: "dom"
    };
    return c;
  }

  // Innermost elements whose attributes mention a token and whose text is short enough to be that field.
  function byAttribute(re, maxLen) {
    var hits = [];
    for (var i = 0; i < all.length; i++) {
      var e = all[i];
      var at = attrText(e);
      if (!at || !re.test(at)) continue;
      var t = textOf(e);
      if (!t || t.length > maxLen) continue;
      hits.push({ e: e, reason: (re.exec(at) || [""])[0] });
    }
    return hits.filter(function (h) {
      return !hits.some(function (o) { return o !== h && h.e.contains(o.e); });
    });
  }
  function pick(list, extraFor) {
    if (list.length > MAX_CANDIDATES) truncated = true;
    return list.slice(0, MAX_CANDIDATES).map(function (h) { return candidate(h.e, extraFor ? extraFor(h) : { reason: h.reason }); });
  }

  var result = { title: [], company: [], location: [], postedAt: [], description: [] };

  // ---- title: headings first, then elements named like a title
  var titleHits = [];
  var seenTitle = new Set();
  Array.prototype.forEach.call(document.querySelectorAll("h1"), function (e) {
    if (!seenTitle.has(e) && textOf(e)) { seenTitle.add(e); titleHits.push({ e: e, reason: "h1" }); }
  });
  byAttribute(/title|headline|job-?name|position/i, 200).forEach(function (h) {
    if (!seenTitle.has(h.e)) { seenTitle.add(h.e); titleHits.push(h); }
  });
  Array.prototype.forEach.call(document.querySelectorAll("h2"), function (e) {
    if (!seenTitle.has(e) && textOf(e) && titleHits.length < 24) { seenTitle.add(e); titleHits.push({ e: e, reason: "h2" }); }
  });
  result.title = pick(titleHits);

  result.company = pick(byAttribute(/employer|company|organi[sz]ation|firma|arbeitgeber|kompani|hiring|brand/i, 120));
  result.location = pick(byAttribute(/location|place|\bort\b|vendndodh|city|region|address|locality|standort/i, 150));

  // ---- date: a machine date first (the datetime attribute is read, not the text), then elements named like a date
  var dateHits = [];
  Array.prototype.forEach.call(document.querySelectorAll("time[datetime]"), function (e) {
    dateHits.push({ e: e, attribute: "datetime", reason: "time[datetime]" });
  });
  var seenDate = new Set(dateHits.map(function (h) { return h.e; }));
  byAttribute(/posted|published|datum|\bdate\b|\bdata\b|created|veröffentlicht/i, 80).forEach(function (h) {
    if (!seenDate.has(h.e)) dateHits.push(h);
  });
  result.postedAt = pick(dateHits, function (h) { return { attribute: h.attribute || "", reason: h.reason }; });

  // ---- description: the large boxes, with the furniture inside them as suggestions to leave out
  var pageText = textOf(document.body).length;
  var DESC = /descri|detail|content|job-?ad|posting|body|text|summary|about/i;
  var FURN = /cookie|consent|share|social|related|similar|apply|banner|newsletter|alert|breadcrumb|recommend|ähnlich|teilen|bewerb/i;
  var boxes = [];
  for (var i = 0; i < all.length; i++) {
    var e = all[i];
    if (!/^(div|section|article|main|td)$/i.test(e.tagName)) continue;
    var len = textOf(e).length;
    if (len < 300 || len >= pageText * 0.92) continue;
    if (e.querySelector("header,nav,footer")) continue;
    if (furnitureOf(e) === "consent") continue;
    var byAttr = DESC.test(attrText(e));
    if (!byAttr && !/^(article|main)$/i.test(e.tagName)) continue;
    boxes.push({ e: e, len: len, byAttr: byAttr });
  }
  boxes.sort(function (a, b) { return b.len - a.len; });
  if (boxes.length > MAX_CANDIDATES) truncated = true;
  result.description = boxes.slice(0, MAX_CANDIDATES).map(function (b) {
    var c = candidate(b.e, { reason: b.byAttr ? "id/class names it" : b.e.tagName.toLowerCase() });
    c.chars = b.len;
    c.shareOfPage = pageText ? Math.round((b.len / pageText) * 100) / 100 : 0;
    c.byAttr = b.byAttr;
    c.headings = b.e.querySelectorAll("h1,h2,h3,h4,h5,h6").length;
    c.listItems = b.e.querySelectorAll("li").length;
    c.paragraphs = b.e.querySelectorAll("p").length;
    var furn = [];
    var inner = b.e.getElementsByTagName("*");
    for (var k = 0; k < inner.length && k < 4000; k++) {
      var x = inner[k];
      if (SKIP.test(x.tagName)) continue;
      var at = attrText(x);
      var kind = FURN.test(at) ? (FURN.exec(at) || [""])[0].toLowerCase() : /^(nav|footer|aside|button|form)$/i.test(x.tagName) ? x.tagName.toLowerCase() : "";
      if (kind) furn.push({ e: x, kind: kind });
    }
    furn = furn.filter(function (f) { return !furn.some(function (o) { return o !== f && o.e.contains(f.e); }); });
    c.furniture = furn.slice(0, MAX_CANDIDATES).map(function (f) {
      var s = selectorOf(f.e);
      return { selector: s.s, unique: s.unique, kind: f.kind, tag: f.e.tagName.toLowerCase(), chars: textOf(f.e).length, value: preview(textOf(f.e)) };
    });
    var low = textOf(b.e).toLowerCase();
    c.lineSignals = ["similar jobs", "ähnliche", "apply now", "jetzt bewerben", "share this", "teilen", "bewerben"].filter(function (k2) { return low.indexOf(k2) >= 0; });
    return c;
  });

  // ---- structured data the page states about the job
  var structured = { jsonld: [], meta: {}, microdata: 0 };
  var ldBytes = 0;
  var blocks = document.querySelectorAll('script[type="application/ld+json"]');
  var jobs = [];
  function walk(n, depth) {
    if (!n || typeof n !== "object" || depth > 8) return;
    if (Array.isArray(n)) { n.slice(0, 200).forEach(function (x) { walk(x, depth + 1); }); return; }
    var t = n["@type"];
    if (t === "JobPosting" || (Array.isArray(t) && t.indexOf("JobPosting") >= 0)) jobs.push(n);
    Object.keys(n).slice(0, 100).forEach(function (k) { walk(n[k], depth + 1); });
  }
  for (var b = 0; b < blocks.length; b++) {
    if (b >= MAX_LD_BLOCKS) { truncated = true; break; }
    var raw = blocks[b].textContent || "";
    ldBytes += raw.length;
    if (ldBytes > MAX_LD_BYTES) { truncated = true; break; }
    try { walk(JSON.parse(raw), 0); } catch (err) {}
  }
  function str(v) { return typeof v === "string" ? v : v && typeof v === "object" && typeof v.name === "string" ? v.name : ""; }
  jobs.slice(0, 3).forEach(function (j) {
    var loc = Array.isArray(j.jobLocation) ? j.jobLocation[0] : j.jobLocation;
    var ad = loc && loc.address;
    var place = ad && typeof ad === "object"
      ? [ad.addressLocality, ad.addressRegion, str(ad.addressCountry)].filter(function (x) { return typeof x === "string" && x; }).join(", ")
      : typeof ad === "string" ? ad : "";
    var desc = typeof j.description === "string" ? norm(j.description.replace(/<[^>]*>/g, " ").replace(/&nbsp;/g, " ")) : "";
    structured.jsonld.push({
      type: "JobPosting",
      title: preview(str(j.title)),
      company: preview(str(j.hiringOrganization)),
      location: preview(place),
      datePosted: preview(typeof j.datePosted === "string" ? j.datePosted : ""),
      descriptionChars: desc.length,
      descriptionPreview: preview(desc)
    });
  });
  var meta = function (n) { var m = document.querySelector('meta[property="' + n + '"],meta[name="' + n + '"]'); return m ? preview(m.getAttribute("content") || "") : ""; };
  structured.meta = { ogTitle: meta("og:title"), ogSiteName: meta("og:site_name"), ogDescription: meta("og:description"), twitterTitle: meta("twitter:title") };
  structured.microdata = document.querySelectorAll('[itemtype*="JobPosting"]').length;

  // ---- every text element, so a person can pick what the heuristics missed
  var elements = [];
  for (var q = 0; q < all.length && elements.length < MAX_ELEMENTS; q++) {
    var el = all[q];
    var own = "";
    for (var ch = el.firstChild; ch; ch = ch.nextSibling) if (ch.nodeType === 3) own += ch.nodeValue;
    own = norm(own);
    if (own.length < 2) continue;
    if (furnitureOf(el) === "consent") continue;
    var full = textOf(el);
    if (full.length > 2000) continue;
    var sel = selectorOf(el);
    elements.push({ selector: sel.s, unique: sel.unique, tag: el.tagName.toLowerCase(), text: preview(full), chars: full.length });
  }
  if (elements.length >= MAX_ELEMENTS) truncated = true;

  return JSON.stringify({ candidates: result, structured: structured, elements: elements, pageTextChars: pageText, truncated: truncated });
})()
