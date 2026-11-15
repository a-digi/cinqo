// postcss-layer-variants: keeps responsive and state variants working when several Tailwind stylesheets share ONE page.
//
// The problem. The Cinqo app and every tool each ship their own complete Tailwind stylesheet, and all of them end up on the same page. Inside one stylesheet
// Tailwind sorts a variant (`lg:flex-row`, `hover:bg-x`, `sm:hidden`) AFTER the plain utilities, so `flex-col lg:flex-row` works. But all stylesheets put their
// rules in the same cascade layer (`utilities`) at the same specificity, so the LATER stylesheet wins: a tool loaded after another one has its plain
// `.flex-col` placed after the other tool's `@media (width >= 64rem) { .lg\:flex-row }`, and the responsive rule loses. Pages lose their columns, menus and
// breakpoints; which pages break depends on which tool's script happened to run last.
//
// The fix. Move every variant block (a top-level @media, @supports or @container inside `@layer utilities`) into a layer of its own and declare the layers
// in ONE fixed order, in every stylesheet:
//
//   theme, base, components, utilities, tw-state, tw-sm, tw-md, tw-lg, tw-xl, tw-2xl, tw-width, tw-container
//
// The order of layers, not the order of stylesheets, decides then: every plain utility (any tool) < every state variant (any tool) < every `sm:` < `md:` < `lg:`
// ... So a variant always beats a plain utility and a wider breakpoint always beats a narrower one, exactly as inside one Tailwind stylesheet, whichever
// stylesheet loads first. Rules that were not in `utilities` (the tool's own animation classes, preflight, theme variables, @property) are left alone.
//
// Use it after Tailwind in postcss.config: plugins: { '@tailwindcss/postcss': {}, '<path>/postcss-layer-variants.cjs': {} } (or the require()d plugin).
'use strict'

const BASE_ORDER = ['theme', 'base', 'components', 'utilities']
// The variant layers, lowest priority first. Keep this list identical in every project: a stylesheet that loads first fixes the order for the page.
const VARIANT_LAYERS = ['tw-state', 'tw-sm', 'tw-md', 'tw-lg', 'tw-xl', 'tw-2xl', 'tw-width', 'tw-container']

// Tailwind's default breakpoints, in rem, and the layer each one gets.
const BREAKPOINT_LAYER = { 40: 'tw-sm', 48: 'tw-md', 64: 'tw-lg', 80: 'tw-xl', 96: 'tw-2xl' }

const WIDTH_MIN = /^\(\s*(?:min-width\s*:\s*|width\s*>=\s*)([0-9]*\.?[0-9]+)rem\s*\)$/

// layerFor says which variant layer an at-rule belongs to.
function layerFor(node) {
  if (node.name === 'container') return 'tw-container'
  if (node.name === 'media') {
    const m = WIDTH_MIN.exec(node.params.trim())
    if (m) return BREAKPOINT_LAYER[Number(m[1])] || 'tw-width'
    // any other width condition (an arbitrary min-[..] or a max-*): after the named breakpoints, before nothing else
    if (/\b(?:min-width|max-width|width)\b/.test(node.params)) return 'tw-width'
  }
  // hover, focus-visible groups, dark, motion, contrast, forced-colors, print, orientation, @supports, ...
  return 'tw-state'
}

const VARIANT_AT_RULES = new Set(['media', 'supports', 'container'])

module.exports = () => ({
  postcssPlugin: 'cinqo-layer-variants',
  OnceExit(root, { AtRule }) {
    const buckets = new Map(VARIANT_LAYERS.map((name) => [name, []]))
    let moved = 0
    let lastUtilities = null

    root.each((node) => {
      if (node.type !== 'atrule' || node.name !== 'layer' || node.params.trim() !== 'utilities' || !node.nodes) return
      lastUtilities = node
      for (const child of [...node.nodes]) {
        if (child.type === 'atrule' && VARIANT_AT_RULES.has(child.name)) {
          buckets.get(layerFor(child)).push(child.remove())
          moved++
        }
      }
    })
    if (moved === 0 || lastUtilities === null) return

    // one layer block per used variant layer, after the utilities layer, in the canonical order
    let anchor = lastUtilities
    for (const name of VARIANT_LAYERS) {
      const nodes = buckets.get(name)
      if (nodes.length === 0) continue
      const block = new AtRule({ name: 'layer', params: name, raws: { afterName: ' ', between: ' ' } })
      for (const n of nodes) block.append(n)
      anchor.after(block)
      anchor = block
    }

    // the order statement: `@layer theme, base, components, utilities;` becomes the same plus the variant layers (or is added when there is none)
    const order = [...BASE_ORDER, ...VARIANT_LAYERS].join(', ')
    let statement = null
    root.each((node) => {
      if (statement === null && node.type === 'atrule' && node.name === 'layer' && !node.nodes && /\butilities\b/.test(node.params)) statement = node
    })
    if (statement) {
      statement.params = order
    } else {
      root.prepend(new AtRule({ name: 'layer', params: order }))
    }
  },
})

module.exports.postcss = true
module.exports.VARIANT_LAYERS = VARIANT_LAYERS
module.exports.layerFor = layerFor
