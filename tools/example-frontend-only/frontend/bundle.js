window.__cinqoToolBridge.registerMenuEntry({
  label: 'Example Tool',
  // A plug: a tool that plugs in. Raw SVG; the sidebar keeps only a small allowlist of shapes (path, circle, rect, line... , at most 2000 characters).
  icon: '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor"><path d="M7 3v4M13 3v4" stroke-width="1.3" stroke-linecap="round"/><path d="M5 7h10v3a5 5 0 0 1-10 0V7z" stroke-width="1.3" stroke-linejoin="round"/><path d="M10 15v2.5" stroke-width="1.3" stroke-linecap="round"/></svg>',
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
