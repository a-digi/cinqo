// find_repeating.js (step 94) finds the repeating structures on the currently
// loaded page that look like a list of items (job cards, search results) and
// returns the best few as verified CSS selectors, so an agent can pick a
// listing container without reading 200 KB of HTML. Evaluated as a single
// expression via chromedp.Evaluate, its return value JSON-serialised back into
// Go, same mechanic as extract.js. The placeholder token on the final line is
// replaced with a JSON payload by find_repeating.go before the script runs.
// See plan/ai/tools/browser/step-94-find-repeating-items.md.
(function (payload) {
  var minItems = payload.minItems;
  var maxCandidates = payload.maxCandidates;
  var maxAttributeLength = payload.maxAttributeLength;
  var maxNodes = 30000;
  var sampleHtmlMax = 1500;
  var sampleTextMax = 200;
  var minItemText = 20;

  var SKIP_TAGS = { HEADER: 1, FOOTER: 1, NAV: 1, ASIDE: 1, SCRIPT: 1, STYLE: 1, NOSCRIPT: 1, TEMPLATE: 1, HEAD: 1, SVG: 1, IFRAME: 1 };
  var SKIP_ROLES = { navigation: 1, banner: 1, contentinfo: 1, complementary: 1, progressbar: 1, search: 1 };
  var STABLE_ATTRS = ["data-testid", "data-at", "data-test", "data-qa", "data-cy", "data-automation-id", "data-id-type", "itemtype"];

  var scanned = 0;
  var truncated = false;

  function esc(s) {
    return window.CSS && CSS.escape ? CSS.escape(s) : String(s).replace(/[^a-zA-Z0-9_-]/g, "\\$&");
  }

  function collapse(s) {
    return (s || "").replace(/\s+/g, " ").trim();
  }

  function isHidden(el) {
    if (el.hidden || el.getAttribute("aria-hidden") === "true") return true;
    var cs = window.getComputedStyle(el);
    if (cs.display === "contents") return false;
    if (cs.display === "none" || cs.visibility === "hidden") return true;
    var r = el.getBoundingClientRect();
    return r.width === 0 && r.height === 0;
  }

  // skipped: elements that are never part of a result list — page chrome,
  // navigation, hidden or loading-skeleton content.
  function skipped(el) {
    if (SKIP_TAGS[el.tagName.toUpperCase()]) return true;
    var role = el.getAttribute("role");
    if (role && SKIP_ROLES[role]) return true;
    return isHidden(el);
  }

  function classTokens(el) {
    var c = el.getAttribute("class");
    return c ? c.split(/\s+/).filter(Boolean) : [];
  }

  function signature(el) {
    return el.tagName.toLowerCase() + "." + classTokens(el).sort().join(".");
  }

  function usableHref(a) {
    var h = a.getAttribute("href") || "";
    return !!h && h.charAt(0) !== "#" && !/^javascript:/i.test(h) && !/^mailto:|^tel:/i.test(h);
  }

  // primaryLink is the member's main link: the one whose text is longest (a
  // job card's title link, not the logo or company link that comes first and
  // is shared between cards). "" when the member has no usable link.
  function primaryLink(el) {
    var best = "";
    var bestLen = -1;
    var as = el.querySelectorAll("a[href]");
    var list = [];
    if (el.tagName.toUpperCase() === "A") list.push(el);
    for (var i = 0; i < as.length; i++) list.push(as[i]);
    for (var j = 0; j < list.length; j++) {
      if (!usableHref(list[j])) continue;
      var len = collapse(list[j].innerText || list[j].textContent).length;
      if (len > bestLen) { best = list[j].href; bestLen = len; }
    }
    return best;
  }

  // textOf is the member's visible text: innerText leaves out script and
  // style content, which textContent would include.
  function textOf(el) {
    return collapse(el.innerText !== undefined ? el.innerText : el.textContent);
  }

  // stableAttr returns an attribute selector for a data-* attribute with a short value.
  function stableAttr(el) {
    for (var i = 0; i < STABLE_ATTRS.length; i++) {
      var v = el.getAttribute(STABLE_ATTRS[i]);
      if (v && v.length <= 40) return "[" + STABLE_ATTRS[i] + '="' + v.replace(/"/g, '\\"') + '"]';
    }
    return "";
  }

  // simple is one element's own compact selector (used for ancestors).
  function simple(el) {
    if (el.id && /^[A-Za-z][\w-]*$/.test(el.id) && document.querySelectorAll("#" + esc(el.id)).length === 1) return "#" + esc(el.id);
    var a = stableAttr(el);
    var tag = el.tagName.toLowerCase();
    if (a) return tag + a;
    var cls = classTokens(el).slice(0, 2);
    return tag + cls.map(function (c) { return "." + esc(c); }).join("");
  }

  // memberSelector builds a selector from what every member shares.
  function memberSelector(members) {
    var tag = members[0].tagName.toLowerCase();
    var attr = null;
    for (var i = 0; i < STABLE_ATTRS.length && attr === null; i++) {
      var v = members[0].getAttribute(STABLE_ATTRS[i]);
      if (!v || v.length > 40) continue;
      var all = true;
      for (var j = 1; j < members.length; j++) {
        if (members[j].getAttribute(STABLE_ATTRS[i]) !== v) { all = false; break; }
      }
      if (all) attr = "[" + STABLE_ATTRS[i] + '="' + v.replace(/"/g, '\\"') + '"]';
    }
    if (attr) return tag + attr;
    var common = classTokens(members[0]);
    for (var k = 1; k < members.length && common.length; k++) {
      var set = {};
      classTokens(members[k]).forEach(function (c) { set[c] = 1; });
      common = common.filter(function (c) { return set[c]; });
    }
    return tag + common.map(function (c) { return "." + esc(c); }).join("");
  }

  function sameSet(nodeList, members) {
    if (nodeList.length !== members.length) return false;
    var set = new Set(members);
    for (var i = 0; i < nodeList.length; i++) if (!set.has(nodeList[i])) return false;
    return true;
  }

  // verifiedSelector returns a selector that, run on the live page, selects
  // exactly these members; tightened with ancestors when the plain one also
  // matches elsewhere. "" when none can be made exact.
  function verifiedSelector(members) {
    var base = memberSelector(members);
    var tries = [base];
    var anc = members[0].parentElement;
    var prefix = "";
    for (var depth = 0; depth < 4 && anc && anc !== document.documentElement; depth++) {
      prefix = simple(anc) + " > " + prefix;
      tries.push(prefix + base);
      anc = anc.parentElement;
    }
    for (var i = 0; i < tries.length; i++) {
      var found;
      try { found = document.querySelectorAll(tries[i]); } catch (e) { continue; }
      if (sameSet(found, members)) return tries[i];
    }
    return "";
  }

  function sampleHtml(el) {
    var clone = el.cloneNode(true);
    var junk = clone.querySelectorAll("script,style,svg,noscript,template,img,picture,source,video,iframe");
    for (var i = 0; i < junk.length; i++) junk[i].remove();
    var all = clone.querySelectorAll("*");
    var nodes = [clone];
    for (var j = 0; j < all.length; j++) nodes.push(all[j]);
    nodes.forEach(function (n) {
      Array.prototype.slice.call(n.attributes).forEach(function (a) {
        // Presentation and accessibility noise never helps pick a selector.
        if (a.name === "style" || a.name === "tabindex" || a.name === "role" || a.name === "target" || a.name === "rel" || a.name.indexOf("aria-") === 0) {
          n.removeAttribute(a.name);
          return;
        }
        if (a.name === "href") {
          if (/^data:/i.test(a.value)) n.removeAttribute(a.name);
          return;
        }
        if (maxAttributeLength > 0 && a.value.length > maxAttributeLength) n.removeAttribute(a.name);
      });
    });
    var html = clone.outerHTML.replace(/\s+/g, " ");
    return html.length > sampleHtmlMax ? html.slice(0, sampleHtmlMax) + "…" : html;
  }

  function describe(members) {
    var withLink = 0;
    var hrefs = {};
    var textLen = 0;
    var textMembers = 0;
    var firstLinks = [];
    var sampleEl = null;
    members.forEach(function (m) {
      var link = primaryLink(m);
      var t = textOf(m);
      textLen += t.length;
      if (t.length >= minItemText) textMembers++;
      if (link) {
        withLink++;
        hrefs[link] = 1;
        if (firstLinks.length < 3 && firstLinks.indexOf(link) < 0) firstLinks.push(link);
        if (!sampleEl && t.length >= minItemText) sampleEl = m;
      }
    });
    return {
      withLink: withLink,
      distinctLinks: Object.keys(hrefs).length,
      textMembers: textMembers,
      avgText: textLen / members.length,
      firstLinks: firstLinks,
      sampleEl: sampleEl || members[0],
    };
  }

  var groups = [];

  function consider(parent, members) {
    if (members.length < minItems) return;
    var d = describe(members);
    var need = Math.ceil(members.length * 0.6);
    if (d.withLink < need || d.textMembers < need) return;
    var score = members.length * (d.withLink / members.length) * (d.distinctLinks / members.length) * Math.min(3, 1 + d.avgText / 80);
    groups.push({ members: members, stats: d, score: score });
  }

  function walk(el) {
    if (++scanned > maxNodes) { truncated = true; return; }
    var kids = [];
    for (var i = 0; i < el.children.length; i++) {
      if (!skipped(el.children[i])) kids.push(el.children[i]);
    }
    if (kids.length >= minItems) {
      var bySig = {};
      var byTag = {};
      kids.forEach(function (k) {
        var s = signature(k);
        (bySig[s] = bySig[s] || []).push(k);
        var t = k.tagName.toLowerCase();
        (byTag[t] = byTag[t] || []).push(k);
      });
      var seen = {};
      Object.keys(bySig).forEach(function (s) {
        consider(el, bySig[s]);
        seen[bySig[s].length + ":" + bySig[s][0].tagName] = 1;
      });
      // Items whose class lists differ a little (a "featured" badge class on
      // some): group by tag when one tag dominates the children.
      Object.keys(byTag).forEach(function (t) {
        if (byTag[t].length >= kids.length * 0.8 && !seen[byTag[t].length + ":" + byTag[t][0].tagName]) consider(el, byTag[t]);
      });
    }
    for (var j = 0; j < kids.length && !truncated; j++) walk(kids[j]);
  }

  // --- next-page control candidates (step 95) -------------------------------
  // crawl_paginated advances by clicking `pagination.nextSelector`; the agent
  // otherwise has to guess one. Found here by what the control says it is
  // (rel=next, an aria-label or attribute naming "next", an arrow or "Weiter"
  // text, the element after the current page number), each as a selector
  // verified on the live page.
  var NEXT_TEXT = /^(next|next page|weiter|nächste|nächste seite|naechste|suivant|page suivante|siguiente|volgende|avanti|seguinte|[›»→>]|>>)$/i;
  var NEXT_HINT = /(^|[\s_\-:.])(next|weiter|naechste|nächste|suivant|siguiente|volgende|avanti)([\s_\-:.]|$)/i;
  var PAGINATION = /pagin|pager|page-nav|pagenav|seitennavigation/i;

  function attrText(el) {
    return [el.id, el.getAttribute("class"), el.getAttribute("role"), el.getAttribute("aria-label"), el.getAttribute("data-at"), el.getAttribute("data-testid"), el.getAttribute("data-test")]
      .filter(Boolean)
      .join(" ");
  }

  function inPagination(el) {
    for (var a = el.parentElement, i = 0; a && i < 8; a = a.parentElement, i++) {
      if (PAGINATION.test(attrText(a))) return true;
    }
    return false;
  }

  function isDisabledControl(el) {
    if (el.disabled || el.getAttribute("aria-disabled") === "true") return true;
    return /(^|\s)(disabled|is-disabled|inactive)(\s|$)/i.test(el.getAttribute("class") || "");
  }

  function usableControl(el) {
    if (!el || el.nodeType !== 1 || isDisabledControl(el) || isHidden(el)) return false;
    var tag = el.tagName.toUpperCase();
    return tag === "A" ? usableHref(el) : tag === "BUTTON" || el.getAttribute("role") === "button" || /(^|\s)next(\s|$)/i.test(el.getAttribute("rel") || "");
  }

  function controlLabel(el) {
    return collapse(el.getAttribute("aria-label") || el.getAttribute("title") || "");
  }

  function nextScore(el) {
    var rel = el.getAttribute("rel") || "";
    if (/(^|\s)next(\s|$)/i.test(rel)) return { score: 3, reason: "rel=next" };
    var paged = inPagination(el);
    var text = collapse(el.innerText || el.textContent).slice(0, 40);
    var hint = attrText(el);
    if (NEXT_HINT.test(hint) && (paged || /next|weiter/i.test(el.getAttribute("data-at") || el.getAttribute("data-testid") || ""))) {
      return { score: 2.5 + (paged ? 0.3 : 0), reason: "an attribute, class or label says next" };
    }
    if (NEXT_HINT.test(controlLabel(el)) && paged) return { score: 2.2, reason: "aria-label or title says next" };
    if (NEXT_TEXT.test(text) && (paged || text.length <= 2)) return { score: 2 + (paged ? 0.3 : 0), reason: "text says next" };
    return null;
  }

  // currentMarker is a position-independent selector for the current-page
  // item (the marker moves as pages advance, a plain nth-child would not).
  function currentMarker(cur, node) {
    var tag = node.tagName.toLowerCase();
    if (node === cur) {
      var ac = cur.getAttribute("aria-current");
      if (ac === "page" || ac === "true") return tag + '[aria-current="' + ac + '"]';
    } else {
      var inner = cur.getAttribute("aria-current");
      if (inner === "page" || inner === "true") return tag + ':has([aria-current="' + inner + '"])';
    }
    var tokens = classTokens(node);
    for (var k = 0; k < tokens.length; k++) {
      if (/^(active|current|selected|is-active|is-current|is-selected)$/i.test(tokens[k])) return tag + "." + esc(tokens[k]);
    }
    return "";
  }

  function paginationAncestor(el) {
    for (var a = el.parentElement, i = 0; a && i < 8; a = a.parentElement, i++) {
      if (PAGINATION.test(attrText(a))) return simple(a);
    }
    return "";
  }

  // followsCurrent: in a pagination container, the control right after the
  // current page number, found with a selector that always means "the control
  // after the current page" (current marker + adjacent sibling), verified to
  // select exactly that control. A positional selector would click the same
  // page again from page 3 on, so it is never offered.
  function followsCurrent() {
    var out = [];
    var currents = document.querySelectorAll('[aria-current="page"], [aria-current="true"], .active, .current, .selected');
    for (var i = 0; i < currents.length; i++) {
      var cur = currents[i];
      if (!inPagination(cur) && !PAGINATION.test(attrText(cur))) continue;
      var node = cur;
      for (var up = 0; up < 3 && node; up++) {
        var sib = node.nextElementSibling;
        if (!sib) { node = node.parentElement; continue; }
        var ctl = sib.matches("a[href], button") ? sib : sib.querySelector("a[href], button");
        var marker = ctl ? currentMarker(cur, node) : "";
        if (ctl && marker) {
          var rel = marker + " + " + sib.tagName.toLowerCase() + (ctl === sib ? "" : " " + ctl.tagName.toLowerCase());
          var container = paginationAncestor(cur);
          var tries = container ? [container + " " + rel, rel] : [rel];
          for (var t = 0; t < tries.length; t++) {
            var found;
            try { found = Array.prototype.slice.call(document.querySelectorAll(tries[t])); } catch (e) { continue; }
            if (found.length === 1 && found[0] === ctl) { out.push({ el: ctl, selector: tries[t] }); break; }
          }
        }
        break;
      }
    }
    return out;
  }

  // nextBases lists the selectors to try for one control, most stable first: a
  // rel=next attribute, a stable data-* attribute, its aria-label or title (a
  // word like "Nächste" survives a redeploy; a generated class does not), and
  // only then its classes.
  function nextBases(el) {
    var tag = el.tagName.toLowerCase();
    var out = [];
    if (/(^|\s)next(\s|$)/i.test(el.getAttribute("rel") || "")) out.push(tag + '[rel~="next"]');
    var stable = stableAttr(el);
    if (stable) out.push(tag + stable);
    ["aria-label", "title"].forEach(function (name) {
      var v = el.getAttribute(name);
      if (v && v.length <= 40) out.push(tag + "[" + name + '="' + v.replace(/"/g, '\\"') + '"]');
    });
    var plain = memberSelector([el]);
    out.push(plain);
    // A bare arrow with nothing to tell it apart from the "previous" arrow
    // beside it: the next control is the last one in its bar.
    out.push(plain + ":last-child");
    out.push(tag + ":last-child");
    return out;
  }

  function exactNext(el, foundSet) {
    var bases = nextBases(el);
    for (var b = 0; b < bases.length; b++) {
      var tries = [bases[b]];
      var anc = el.parentElement;
      var prefix = "";
      for (var depth = 0; depth < 4 && anc && anc !== document.documentElement; depth++) {
        if (depth === 0) tries.push(simple(anc) + ":last-child > " + bases[b]);
        prefix = simple(anc) + " > " + prefix;
        tries.push(prefix + bases[b]);
        anc = anc.parentElement;
      }
      for (var i = 0; i < tries.length; i++) {
        var found;
        try { found = Array.prototype.slice.call(document.querySelectorAll(tries[i])); } catch (e) { continue; }
        if (found.length && found.indexOf(el) >= 0 && found.every(function (f) { return foundSet.has(f); })) {
          return { selector: tries[i], matches: found.length };
        }
      }
    }
    return null;
  }

  function findNextControls() {
    var scored = [];
    var seen = new Set();
    var all = document.querySelectorAll('a[href], button, [role="button"], [rel~="next"]');
    for (var i = 0; i < all.length && i < 6000; i++) {
      var el = all[i];
      if (seen.has(el) || !usableControl(el)) continue;
      var sc = nextScore(el);
      if (sc) { seen.add(el); scored.push({ el: el, score: sc.score, reason: sc.reason, order: i }); }
    }
    followsCurrent().forEach(function (f, k) {
      if (!usableControl(f.el)) return;
      // Already found by what it says (e.g. an aria-label): that entry stays,
      // its selector is the position-independent one only when it has none.
      if (seen.has(f.el)) {
        scored.forEach(function (c) { if (c.el === f.el && !c.dynamic) c.dynamic = f.selector; });
        return;
      }
      seen.add(f.el);
      scored.push({ el: f.el, score: 1.8, reason: "follows the current page number", order: 100000 + k, dynamic: f.selector });
    });
    var foundSet = new Set(scored.map(function (c) { return c.el; }));
    scored.sort(function (a, b) { return b.score - a.score || a.order - b.order; });
    var out = [];
    var selectors = {};
    for (var j = 0; j < scored.length && out.length < 3; j++) {
      var c = scored[j];
      var ex = c.reason === "follows the current page number" ? { selector: c.dynamic, matches: 1 } : exactNext(c.el, foundSet);
      if (!ex || !ex.selector || selectors[ex.selector]) continue;
      selectors[ex.selector] = 1;
      out.push({
        selector: ex.selector,
        matches: ex.matches,
        text: collapse(c.el.innerText || c.el.textContent).slice(0, 40),
        label: controlLabel(c.el).slice(0, 60),
        href: c.el.tagName.toUpperCase() === "A" ? c.el.href : "",
        reason: c.reason,
      });
    }
    return out;
  }

  function nextPageUrl() {
    var l = document.querySelector('link[rel~="next"][href]');
    if (l) return l.href;
    var a = document.querySelector('a[rel~="next"][href]');
    return a ? a.href : "";
  }

  var root = document.body || document.documentElement;
  if (!root) return { candidates: [], nextCandidates: [], nextPageUrl: "", truncated: false };
  walk(root);

  groups.sort(function (a, b) { return b.score - a.score; });

  var chosen = [];
  var seenSelectors = {};
  for (var gi = 0; gi < groups.length && chosen.length < maxCandidates; gi++) {
    var g = groups[gi];
    // A sub-structure inside the members of a higher-ranked candidate (the
    // tags inside every card) is noise, not another list.
    var nested = chosen.some(function (c) {
      return g.members.every(function (m) {
        return c.members.some(function (cm) { return cm !== m && cm.contains(m); });
      });
    });
    if (nested) continue;
    var selector = verifiedSelector(g.members);
    if (!selector || seenSelectors[selector]) continue;
    seenSelectors[selector] = 1;
    var sampleEl = g.stats.sampleEl;
    chosen.push({
      members: g.members,
      selector: selector,
      count: g.members.length,
      membersWithLink: g.stats.withLink,
      distinctLinks: g.stats.distinctLinks,
      firstLinks: g.stats.firstLinks,
      sampleText: textOf(sampleEl).slice(0, sampleTextMax),
      sampleHtml: sampleHtml(sampleEl),
    });
  }

  var nextCandidates = [];
  try { nextCandidates = findNextControls(); } catch (e) { nextCandidates = []; }

  return {
    nextCandidates: nextCandidates,
    nextPageUrl: nextPageUrl(),
    candidates: chosen.map(function (c) {
      return {
        selector: c.selector,
        count: c.count,
        membersWithLink: c.membersWithLink,
        distinctLinks: c.distinctLinks,
        firstLinks: c.firstLinks,
        sampleText: c.sampleText,
        sampleHtml: c.sampleHtml,
      };
    }),
    truncated: truncated,
  };
})(__FIND_REPEATING_PAYLOAD__);
