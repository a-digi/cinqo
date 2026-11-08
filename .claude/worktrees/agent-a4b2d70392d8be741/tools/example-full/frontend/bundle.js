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
