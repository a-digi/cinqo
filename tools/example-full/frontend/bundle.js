window.__cinqoToolBridge.registerMenuEntry({
  label: 'Example Full Tool',
  path: '/tools/example_full/home',
  scopes: ['tool:example_full:read'],
})

// The id of the example item the comments below are about. In a real tool this is the
// UUID of whatever the page shows (a job, an invoice, a ticket…); comments belong to
// (your tool, an entity type from manifest.json's "comments", that UUID).
var EXAMPLE_ITEM_ID = '8d3c5b1e-2f4a-4c6d-9e7f-0a1b2c3d4e5f'

window.__cinqoToolBridge.registerRoute({
  path: '/tools/example_full/home',
  mount: function (container) {
    var heading = document.createElement('h1')
    heading.textContent = 'Example Full Tool'
    var items = document.createElement('pre')
    items.textContent = 'Loading items from its own backend…'
    var commentsBox = document.createElement('div')
    commentsBox.style.marginTop = '24px'
    container.appendChild(heading)
    container.appendChild(items)
    container.appendChild(commentsBox)

    fetch('/api/v1/tools/example_full/proxy/items', { credentials: 'include' })
      .then(function (r) {
        return r.json()
      })
      .then(function (data) {
        items.textContent = JSON.stringify(data, null, 2)
      })

    // Comments: Cinqo draws the whole panel (list, add, edit, delete, filters) and
    // decides who may do what from the scopes in manifest.json. Older hosts do not have
    // this bridge method, so check for it. It returns a function that removes the panel.
    var bridge = window.__cinqoToolBridge
    if (typeof bridge.mountComments === 'function') {
      container.__unmountComments = bridge.mountComments(commentsBox, {
        owner: 'example_full', // this tool's own slug
        entityType: 'item', // declared in manifest.json "comments"
        entityId: EXAMPLE_ITEM_ID,
        title: 'Comments on this item',
      })
    }
  },
  unmount: function (container) {
    if (container.__unmountComments) {
      container.__unmountComments()
      delete container.__unmountComments
    }
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
