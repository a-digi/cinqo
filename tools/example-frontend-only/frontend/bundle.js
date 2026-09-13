window.__cinqoToolBridge.registerMenuEntry({
  label: 'Example Tool',
  path: '/tools/example_frontend_only/home',
  scopes: ['tool:example_frontend_only:read'],
})

window.__cinqoToolBridge.registerRoute({
  path: '/tools/example_frontend_only/home',
  mount: function (container) {
    container.innerHTML = '<h1>Example Frontend-Only Tool</h1><p>Replace this with your own UI.</p>'
  },
  unmount: function (container) {
    container.innerHTML = ''
  },
})
