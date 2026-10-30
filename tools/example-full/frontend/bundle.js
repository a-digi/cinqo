window.__cinqoToolBridge.registerMenuEntry({
  label: 'Example Full Tool',
  path: '/tools/example_full/home',
  scopes: ['tool:example_full:read'],
})

window.__cinqoToolBridge.registerRoute({
  path: '/tools/example_full/home',
  mount: function (container) {
    container.innerHTML = '<h1>Example Full Tool</h1><p>Loading items from its own backend…</p>'
    fetch('/api/v1/tools/example_full/proxy/items', { credentials: 'include' })
      .then(function (r) {
        return r.json()
      })
      .then(function (data) {
        container.innerHTML = '<h1>Example Full Tool</h1><pre>' + JSON.stringify(data, null, 2) + '</pre>'
      })
  },
  unmount: function (container) {
    container.innerHTML = ''
  },
})

// A "custom" Home dashboard widget (manifest.json: widgets[].kind = "custom").
// Cinqo fetches this widget's data from the tool's own backend route and hands it
// to mount(); we draw it. The id is "<tool slug>.<widget key>". Tool bundles cannot
// use Tailwind classes — use inline styles and the --cinqo-tool-* CSS variables,
// each with a literal fallback. Put data in with textContent (never innerHTML).
window.__cinqoToolBridge.registerDashboardWidget({
  id: 'example_full.custom_demo',
  mount: function (container, data) {
    var box = document.createElement('div')
    box.style.fontFamily = 'var(--cinqo-tool-font, system-ui, sans-serif)'
    box.style.color = 'var(--cinqo-tool-text, #111827)'
    var heading = document.createElement('p')
    heading.style.fontWeight = '600'
    heading.style.margin = '0 0 8px'
    var list = document.createElement('ul')
    list.style.margin = '0'
    list.style.paddingLeft = '18px'
    list.style.color = 'var(--cinqo-tool-text-muted, #6b7280)'
    box.appendChild(heading)
    box.appendChild(list)
    container.appendChild(box)
    container.__render = function (next) {
      heading.textContent = String(next.message || '')
      list.replaceChildren()
      ;(Array.isArray(next.items) ? next.items : []).forEach(function (item) {
        var li = document.createElement('li')
        li.textContent = String(item)
        list.appendChild(li)
      })
    }
    container.__render(data)
  },
  // Optional: called when fresh data arrives. Leave it out and Cinqo redraws
  // by calling unmount() and then mount() again.
  update: function (container, data) {
    container.__render(data)
  },
  unmount: function (container) {
    delete container.__render
  },
})
