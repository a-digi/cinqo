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
//
// Styling: this bundle is installed at runtime and never passes
// through the app's own Tailwind build, so it can never use a
// Tailwind utility class — styled instead via the app's own
// --cinqo-tool-* CSS custom properties (frontend/src/index.css,
// documented in toolBridge.ts), each referenced with a literal
// fallback so the page still renders sensibly even if they're ever
// missing. See
// plan/ai/tools/browser/step-12-menu-grouping-and-tool-styling-contract.md.
(function () {
  var PROXY_BASE = '/api/v1/tools/browser/proxy/login-credentials'

  window.__cinqoToolBridge.registerMenuEntry({
    label: 'Browser',
    children: [
      {
        label: 'Login',
        path: '/tools/browser/credentials',
        // Reuses the existing tool:browser:login scope — the same
        // permission that already gates writing a credential via the
        // underlying route, deliberately not a separate scope: a user
        // who can't submit a credential shouldn't see a page whose
        // only purpose is submitting one.
        scopes: ['tool:browser:login'],
      },
    ],
  })

  window.__cinqoToolBridge.registerRoute({
    path: '/tools/browser/credentials',
    mount: mount,
    unmount: function (container) {
      container.innerHTML = ''
    },
  })

  var STYLE_ID = 'cinqo-browser-credentials-style'

  function ensureStyle() {
    if (document.getElementById(STYLE_ID)) return
    var style = document.createElement('style')
    style.id = STYLE_ID
    style.textContent =
      '.cinqo-browser-credentials {' +
      '  max-width: 680px;' +
      '  padding: 24px 0;' +
      '  font-family: var(--cinqo-tool-font, system-ui, sans-serif);' +
      '  color: var(--cinqo-tool-text, #111827);' +
      '}' +
      '.cinqo-browser-credentials h1 {' +
      '  font-size: 1.25rem;' +
      '  margin: 0 0 6px;' +
      '}' +
      '.cinqo-browser-credentials h2 {' +
      '  font-size: 1rem;' +
      '  margin: 32px 0 14px;' +
      '}' +
      '.cinqo-browser-credentials p.description {' +
      '  color: var(--cinqo-tool-text-muted, #6b7280);' +
      '  font-size: 0.875rem;' +
      '  margin: 0 0 20px;' +
      '}' +
      '.cinqo-browser-credentials [data-role="status"] {' +
      '  min-height: 1.2em;' +
      '  color: var(--cinqo-tool-danger, #b91c1c);' +
      '  font-size: 0.875rem;' +
      '}' +
      '.cinqo-browser-credentials .card {' +
      '  border: 1px solid var(--cinqo-tool-border, #e5e7eb);' +
      '  border-radius: var(--cinqo-tool-radius, 6px);' +
      '  box-shadow: var(--cinqo-tool-shadow, 0 1px 2px 0 rgb(0 0 0 / 0.05));' +
      '  overflow: hidden;' +
      '}' +
      '.cinqo-browser-credentials table {' +
      '  width: 100%;' +
      '  border-collapse: collapse;' +
      '  font-size: 0.875rem;' +
      '}' +
      '.cinqo-browser-credentials th {' +
      '  text-align: left;' +
      '  padding: 12px;' +
      '  background: var(--cinqo-tool-surface, #f9fafb);' +
      '  border-bottom: 1px solid var(--cinqo-tool-border, #e5e7eb);' +
      '  font-weight: 500;' +
      '  color: var(--cinqo-tool-text-muted, #6b7280);' +
      '}' +
      '.cinqo-browser-credentials td {' +
      '  padding: 12px;' +
      '  border-bottom: 1px solid var(--cinqo-tool-border, #e5e7eb);' +
      '}' +
      '.cinqo-browser-credentials tbody tr:last-child td {' +
      '  border-bottom: none;' +
      '}' +
      '.cinqo-browser-credentials tbody tr:hover {' +
      '  background: var(--cinqo-tool-surface, #f9fafb);' +
      '}' +
      '.cinqo-browser-credentials form.card {' +
      '  padding: 24px;' +
      '  background: var(--cinqo-tool-surface, #f9fafb);' +
      '}' +
      '.cinqo-browser-credentials .field {' +
      '  margin-bottom: 16px;' +
      '}' +
      '.cinqo-browser-credentials label {' +
      '  display: block;' +
      '  font-size: 0.8125rem;' +
      '  font-weight: 500;' +
      '  margin-bottom: 6px;' +
      '}' +
      '.cinqo-browser-credentials input {' +
      '  width: 100%;' +
      '  box-sizing: border-box;' +
      '  padding: 10px 12px;' +
      '  font-size: 0.875rem;' +
      '  background: var(--cinqo-tool-input-bg, #ffffff);' +
      '  border: 1px solid var(--cinqo-tool-border, #e5e7eb);' +
      '  border-radius: var(--cinqo-tool-radius, 6px);' +
      '  font-family: inherit;' +
      '  transition: border-color 120ms ease, box-shadow 120ms ease;' +
      '}' +
      '.cinqo-browser-credentials input:focus {' +
      '  outline: none;' +
      '  border-color: var(--cinqo-tool-primary, #111827);' +
      '  box-shadow: 0 0 0 3px rgb(17 24 39 / 0.1);' +
      '}' +
      '.cinqo-browser-credentials button {' +
      '  font-family: inherit;' +
      '  font-size: 0.875rem;' +
      '  border-radius: var(--cinqo-tool-radius, 6px);' +
      '  cursor: pointer;' +
      '  transition: background-color 120ms ease;' +
      '}' +
      '.cinqo-browser-credentials button[type="submit"] {' +
      '  background: var(--cinqo-tool-primary, #111827);' +
      '  color: var(--cinqo-tool-primary-text, #ffffff);' +
      '  border: none;' +
      '  padding: 10px 18px;' +
      '  font-weight: 500;' +
      '}' +
      '.cinqo-browser-credentials button[type="submit"]:hover {' +
      '  background: var(--cinqo-tool-primary-hover, #1f2937);' +
      '}' +
      '.cinqo-browser-credentials button[data-role="remove"] {' +
      '  background: transparent;' +
      '  color: var(--cinqo-tool-danger, #b91c1c);' +
      '  border: 1px solid var(--cinqo-tool-border, #e5e7eb);' +
      '  padding: 5px 12px;' +
      '}' +
      '.cinqo-browser-credentials button[data-role="remove"]:hover {' +
      '  background: var(--cinqo-tool-danger-hover, #fef2f2);' +
      '}'
    document.head.appendChild(style)
  }

  function mount(container) {
    ensureStyle()

    container.innerHTML =
      '<div class="cinqo-browser-credentials">' +
      '<h1>Login Credentials</h1>' +
      '<p class="description">Domains the Browser tool can log into on your behalf. ' +
      'The AI can ask whether a credential exists for a domain, but never sees the username or password stored here.</p>' +
      '<div data-role="status"></div>' +
      '<div class="card">' +
      '<table data-role="list">' +
      '<thead><tr>' +
      '<th>Domain</th>' +
      '<th>Username</th>' +
      '<th></th>' +
      '</tr></thead>' +
      '<tbody data-role="rows"></tbody>' +
      '</table>' +
      '</div>' +
      '<h2>Add / Update Credential</h2>' +
      '<form class="card" data-role="form">' +
      '<div class="field"><label for="cinqo-browser-cred-domain">Domain</label>' +
      '<input id="cinqo-browser-cred-domain" data-role="domain" type="text" placeholder="example.com" required></div>' +
      '<div class="field"><label for="cinqo-browser-cred-username">Username</label>' +
      '<input id="cinqo-browser-cred-username" data-role="username" type="text" required></div>' +
      '<div class="field"><label for="cinqo-browser-cred-password">Password</label>' +
      '<input id="cinqo-browser-cred-password" data-role="password" type="password" required></div>' +
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
        domainTd.textContent = cred.domain
        tr.appendChild(domainTd)

        var usernameTd = document.createElement('td')
        usernameTd.textContent = cred.username
        tr.appendChild(usernameTd)

        var actionTd = document.createElement('td')
        var removeBtn = document.createElement('button')
        removeBtn.type = 'button'
        removeBtn.setAttribute('data-role', 'remove')
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
