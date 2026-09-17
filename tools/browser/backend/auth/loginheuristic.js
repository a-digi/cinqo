// loginheuristic.js finds probable login form elements on the
// currently loaded page — a plain, deterministic DOM search (no ML/
// embedding-based classification), kept as its own file so it's
// readable/testable on its own rather than an inline Go string
// literal. Evaluated as a single expression via chromedp.Evaluate;
// its return value is JSON-serialized back into Go. See
// plan/ai/tools/browser/step-04-find-login-elements-feature.md.
(function () {
  function cssSelector(el) {
    if (!el) return null;
    if (el.id) return "#" + CSS.escape(el.id);
    var parts = [];
    var cur = el;
    while (cur && cur.nodeType === 1 && parts.length < 5) {
      var part = cur.tagName.toLowerCase();
      if (cur.getAttribute && cur.getAttribute("name")) {
        part += '[name="' + cur.getAttribute("name").replace(/"/g, '\\"') + '"]';
        parts.unshift(part);
        break;
      }
      var parent = cur.parentElement;
      if (parent) {
        var siblings = Array.prototype.filter.call(parent.children, function (c) {
          return c.tagName === cur.tagName;
        });
        if (siblings.length > 1) {
          part += ":nth-of-type(" + (siblings.indexOf(cur) + 1) + ")";
        }
      }
      parts.unshift(part);
      cur = parent;
    }
    return parts.join(" > ");
  }

  function findUsernameField(scope, passwordEl) {
    var inputs = scope.querySelectorAll("input");
    var email = null, text = null, other = null;
    for (var i = 0; i < inputs.length; i++) {
      var inp = inputs[i];
      if (inp === passwordEl) continue;
      var haystack = (inp.name || "") + " " + (inp.id || "") + " " + (inp.autocomplete || "");
      if (!email && inp.type === "email") email = inp;
      else if (!text && inp.type === "text") text = inp;
      else if (!other && /user|email|login/i.test(haystack)) other = inp;
    }
    return email || text || other || null;
  }

  function findSubmit(scope) {
    return (
      scope.querySelector('button[type="submit"]') ||
      scope.querySelector('input[type="submit"]') ||
      scope.querySelector("button") ||
      null
    );
  }

  var results = [];
  var passwordInputs = document.querySelectorAll('input[type="password"]');
  for (var i = 0; i < passwordInputs.length; i++) {
    var pw = passwordInputs[i];
    var form = pw.closest("form");
    var scope = form;
    if (!scope) {
      // No real <form> — widen to a small ancestor container instead
      // of searching the whole document, so an unrelated login widget
      // elsewhere on the page isn't picked up as this one's username
      // field.
      scope = pw;
      for (var d = 0; d < 3 && scope.parentElement; d++) scope = scope.parentElement;
    }

    var username = findUsernameField(scope, pw);
    var submit = findSubmit(scope);

    results.push({
      usernameSelector: username ? cssSelector(username) : null,
      passwordSelector: cssSelector(pw),
      submitSelector: submit ? cssSelector(submit) : null,
      formSelector: form ? cssSelector(form) : null,
      confidence: form ? "high" : "low",
    });
  }
  return results;
})()
