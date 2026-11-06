// The structured data of the loaded page (plan 186, step 10, Phase B): the JobPosting JSON-LD and the meta tags, read by a path. It is
// spliced into extract.js and description_rule.js (one copy of the logic, two scripts). It reads the live page and changes nothing.
//
// structuredValue(source, path) returns the text the path leads to, or null when the page does not say. source "jsonld": path is dotted
// ("title", "hiringOrganization", "jobLocation", "datePosted", "description"); a number is an index into a list, a name that meets a list
// takes its first entry; a value that is an organization or a place becomes its name or its address as "town, region, country". source
// "meta": path is the tag's property or name ("og:title"); the value is its content. The value is raw: JSON-LD text may carry HTML or
// entities (a description does), which the caller turns into text.
var structuredJobDone = false;
var structuredJobValue = null;

function structuredJob() {
  if (structuredJobDone) return structuredJobValue;
  structuredJobDone = true;
  var blocks = document.querySelectorAll('script[type="application/ld+json"]');
  var bytes = 0;
  function walk(n, depth) {
    if (structuredJobValue || !n || typeof n !== "object" || depth > 8) return;
    if (Array.isArray(n)) {
      for (var i = 0; i < n.length && i < 200; i++) walk(n[i], depth + 1);
      return;
    }
    var t = n["@type"];
    if (t === "JobPosting" || (Array.isArray(t) && t.indexOf("JobPosting") >= 0)) {
      structuredJobValue = n;
      return;
    }
    var keys = Object.keys(n);
    for (var k = 0; k < keys.length && k < 100; k++) walk(n[keys[k]], depth + 1);
  }
  for (var b = 0; b < blocks.length && b < 20; b++) {
    var raw = blocks[b].textContent || "";
    bytes += raw.length;
    if (bytes > 400000) break;
    try {
      walk(JSON.parse(raw), 0);
    } catch (e) {}
  }
  return structuredJobValue;
}

function structuredText(v) {
  if (typeof v === "string") {
    var t = v.trim();
    return t === "" ? null : t;
  }
  if (typeof v === "number" || typeof v === "boolean") return String(v);
  if (Array.isArray(v)) {
    for (var i = 0; i < v.length && i < 20; i++) {
      var first = structuredText(v[i]);
      if (first !== null) return first;
    }
    return null;
  }
  if (v && typeof v === "object") {
    if (typeof v.name === "string" && v.name.trim() !== "") return v.name.trim();
    if (v.address) return structuredText(v.address);
    var country = v.addressCountry && typeof v.addressCountry === "object" ? v.addressCountry.name : v.addressCountry;
    var parts = [v.addressLocality, v.addressRegion, country].filter(function (x) {
      return typeof x === "string" && x.trim() !== "";
    });
    return parts.length > 0 ? parts.join(", ") : null;
  }
  return null;
}

function structuredValue(source, path) {
  if (source === "meta") {
    var metas = document.getElementsByTagName("meta");
    for (var i = 0; i < metas.length; i++) {
      var key = metas[i].getAttribute("property") || metas[i].getAttribute("name");
      if (key === path) {
        var c = metas[i].getAttribute("content");
        return c === null || c.trim() === "" ? null : c.trim();
      }
    }
    return null;
  }
  if (source !== "jsonld") return null;
  var cur = structuredJob();
  if (!cur) return null;
  var segs = String(path).split(".");
  for (var s = 0; s < segs.length; s++) {
    var seg = segs[s];
    if (Array.isArray(cur)) {
      if (/^\d+$/.test(seg)) {
        cur = cur[Number(seg)];
        continue;
      }
      cur = cur[0];
    }
    if (cur === null || typeof cur !== "object" || !Object.prototype.hasOwnProperty.call(cur, seg)) return null;
    cur = cur[seg];
  }
  return structuredText(cur);
}
