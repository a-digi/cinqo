// bundle.js is the Browser tool's own frontend — a human-facing page
// for managing stored login credentials, replacing step 7's raw-
// YAML-over-curl workflow with a real in-app page. Plain vanilla JS,
// no build step — matches example-full/example-frontend-only's own
// precedent exactly, and toolBridge.ts's own stated intent ("no React
// import... so a tool author can copy just this file and write their
// bundle against it without pulling in cinqo's own frontend
// toolchain"). See
// plan/ai/tools/browser/step-10-credential-management-frontend.md.
//
// Reuses step 7's GET/POST/DELETE /login-credentials verbatim — no
// backend change was needed for this step. The backend never inspects
// Content-Type on POST (verified directly while designing this step),
// and JSON is valid YAML, so this file posts plain JSON.stringify()
// bodies rather than hand-templating YAML strings — correct escaping
// for free, no injection risk from a domain/username/password
// containing YAML-special characters.
(function () {
  var PROXY_BASE = '/api/v1/tools/browser/proxy/login-credentials'

  window.__cinqoToolBridge.registerMenuEntry({
    label: 'Login Credentials',
    path: '/tools/browser/credentials',
    // Reuses the existing tool:browser:login scope — the same
    // permission that already gates writing a credential via the
    // underlying route, deliberately not a separate scope: a user who
    // can't submit a credential shouldn't see a page whose only
    // purpose is submitting one.
    scopes: ['tool:browser:login'],
  })

  window.__cinqoToolBridge.registerRoute({
    path: '/tools/browser/credentials',
    mount: mount,
    unmount: function (container) {
      container.innerHTML = ''
    },
  })

  function mount(container) {
    container.innerHTML =
      '<div style="max-width:640px;font-family:system-ui,sans-serif">' +
      '<h1>Login Credentials</h1>' +
      '<p style="color:#555">Domains the Browser tool can log into on your behalf. ' +
      'The AI can ask whether a credential exists for a domain, but never sees the username or password stored here.</p>' +
      '<div data-role="status" style="min-height:1.2em;color:#b00"></div>' +
      '<table data-role="list" style="width:100%;border-collapse:collapse;margin:1em 0">' +
      '<thead><tr>' +
      '<th style="text-align:left;border-bottom:1px solid #ccc;padding:4px">Domain</th>' +
      '<th style="text-align:left;border-bottom:1px solid #ccc;padding:4px">Username</th>' +
      '<th style="border-bottom:1px solid #ccc;padding:4px"></th>' +
      '</tr></thead>' +
      '<tbody data-role="rows"></tbody>' +
      '</table>' +
      '<h2 style="font-size:1em">Add / Update Credential</h2>' +
      '<form data-role="form">' +
      '<div style="margin-bottom:8px"><label>Domain<br>' +
      '<input data-role="domain" type="text" placeholder="example.com" style="width:100%;padding:4px" required></label></div>' +
      '<div style="margin-bottom:8px"><label>Username<br>' +
      '<input data-role="username" type="text" style="width:100%;padding:4px" required></label></div>' +
      '<div style="margin-bottom:8px"><label>Password<br>' +
      '<input data-role="password" type="password" style="width:100%;padding:4px" required></label></div>' +
      '<button type="submit">Save</button>' +
      '</form>' +
      '</div>'

    var statusEl = container.querySelector('[data-role="status"]')
    var rowsEl = container.querySelector('[data-role="rows"]')
    var formEl = container.querySelector('[data-role="form"]')
    var domainEl = container.querySelector('[data-role="domain"]')
    var usernameEl = container.querySelector('[data-role="username"]')
    var passwordEl = container.querySelector('[data-role="password"]')

    function setStatus(message) {
      statusEl.textContent = message || ''
    }

    function renderRows(credentials) {
      rowsEl.innerHTML = ''
      for (var i = 0; i < credentials.length; i++) {
        var cred = credentials[i]
        var tr = document.createElement('tr')

        var domainTd = document.createElement('td')
        domainTd.style.padding = '4px'
        domainTd.textContent = cred.domain
        tr.appendChild(domainTd)

        var usernameTd = document.createElement('td')
        usernameTd.style.padding = '4px'
        usernameTd.textContent = cred.username
        tr.appendChild(usernameTd)

        var actionTd = document.createElement('td')
        actionTd.style.padding = '4px'
        var removeBtn = document.createElement('button')
        removeBtn.type = 'button'
        removeBtn.textContent = 'Remove'
        removeBtn.addEventListener('click', function (domain) {
          return function () {
            removeCredential(domain)
          }
        }(cred.domain))
        actionTd.appendChild(removeBtn)
        tr.appendChild(actionTd)

        rowsEl.appendChild(tr)
      }
    }

    function loadCredentials() {
      setStatus('')
      fetch(PROXY_BASE, { credentials: 'include' })
        .then(function (r) {
          if (!r.ok) throw new Error('failed to load credentials (' + r.status + ')')
          return r.json()
        })
        .then(function (data) {
          renderRows(data.credentials || [])
        })
        .catch(function (err) {
          setStatus(err.message)
        })
    }

    function removeCredential(domain) {
      setStatus('')
      fetch(PROXY_BASE + '?domain=' + encodeURIComponent(domain), {
        method: 'DELETE',
        credentials: 'include',
      })
        .then(function (r) {
          if (!r.ok && r.status !== 204) throw new Error('failed to remove credential (' + r.status + ')')
          loadCredentials()
        })
        .catch(function (err) {
          setStatus(err.message)
        })
    }

    formEl.addEventListener('submit', function (event) {
      event.preventDefault()
      setStatus('')

      var domain = normalizeDomain(domainEl.value.trim())
      var username = usernameEl.value
      var password = passwordEl.value

      fetch(PROXY_BASE, {
        method: 'POST',
        credentials: 'include',
        body: JSON.stringify({
          login_credentials: [{ domain: domain, username: username, password: password }],
        }),
      })
        .then(function (r) {
          if (!r.ok) throw new Error('failed to save credential (' + r.status + ')')
          return r.json()
        })
        .then(function (data) {
          // Clear the form's own values explicitly, not just relying on
          // the DOM's default post-submit state — the plaintext
          // password shouldn't linger in the input any longer than
          // necessary.
          formEl.reset()
          renderRows(data.credentials || [])
        })
        .catch(function (err) {
          setStatus(err.message)
        })
    })

    loadCredentials()
  }

  // normalizeDomain lets a user paste a full login URL (e.g.
  // "https://example.com/login") into the domain field instead of
  // requiring a bare hostname — a frontend-only convenience, doesn't
  // touch the backend/schema at all. Falls back to the raw input
  // whenever it isn't a parseable absolute URL.
  function normalizeDomain(raw) {
    try {
      var u = new URL(raw)
      if (u.hostname) return u.hostname
    } catch (e) {
      // Not a parseable absolute URL — treat raw as a bare domain.
    }
    return raw
  }
})()
