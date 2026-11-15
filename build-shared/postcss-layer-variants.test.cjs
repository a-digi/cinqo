'use strict'
// Run: node --test build-shared/   (postcss is taken from the core frontend's node_modules)
const test = require('node:test')
const assert = require('node:assert/strict')
const path = require('node:path')

const postcss = require(require.resolve('postcss', { paths: [path.join(__dirname, '..', 'frontend')] }))
const plugin = require('./postcss-layer-variants.cjs')

const run = async (css) => (await postcss([plugin()]).process(css, { from: undefined })).css
const squash = (s) => s.replace(/\s+/g, ' ').trim()

// What Tailwind 4 writes: layers, the order statement, then one utilities layer holding plain rules and variant blocks.
const SHEET = `
@layer properties { :root { --tw-x: 0; } }
@layer theme { :root { --c: red; } }
@layer base { * { margin: 0; } }
@layer theme, base, components, utilities;
@layer utilities {
  .flex-col { flex-direction: column; }
  @media (hover: hover) { .hover\\:bg-a:hover { background: red; } }
  @media (width >= 64rem) { .lg\\:flex-row { flex-direction: row; } }
  .p-4 { padding: 1rem; }
  @media (width >= 40rem) { .sm\\:block { display: block; } }
  @media (width >= 48rem) { .md\\:flex { display: flex; } }
  @supports (color: color-mix(in lab, red, red)) { .bg-b\\/50 { background: color-mix(in oklab, red 50%, transparent); } }
  @media (prefers-color-scheme: dark) { .dark\\:x { color: white; } }
}
.career-enter { animation: x 1s; }
`

const layerNames = (css) => [...css.matchAll(/@layer ([a-z0-9-]+) \{/g)].map((m) => m[1])

test('a variant block leaves the utilities layer and gets the layer of its kind', async () => {
  const out = await run(SHEET)
  const util = out.slice(out.indexOf('@layer utilities {'), out.indexOf('@layer tw-state {'))
  assert.match(squash(util), /\.flex-col/)
  assert.match(squash(util), /\.p-4/)
  assert.doesNotMatch(util, /@media|@supports/)
  const layer = (name) => squash(out.slice(out.indexOf(`@layer ${name} {`)).split(/\n}\n/)[0])
  assert.match(layer('tw-lg'), /lg\\:flex-row/)
  assert.match(layer('tw-sm'), /sm\\:block/)
  assert.match(layer('tw-md'), /md\\:flex/)
  assert.match(layer('tw-state'), /hover\\:bg-a/)
  assert.match(layer('tw-state'), /bg-b\\\/50/)
  assert.match(layer('tw-state'), /dark\\:x/)
})

test('the layers are declared in the one fixed order, and the blocks follow the utilities layer in that order', async () => {
  const out = await run(SHEET)
  assert.match(out, /@layer theme, base, components, utilities, tw-state, tw-sm, tw-md, tw-lg, tw-xl, tw-2xl, tw-width, tw-container;/)
  assert.deepEqual(layerNames(out).filter((n) => n.startsWith('tw-') || n === 'utilities'), ['utilities', 'tw-state', 'tw-sm', 'tw-md', 'tw-lg'])
  assert.deepEqual(plugin.VARIANT_LAYERS, ['tw-state', 'tw-sm', 'tw-md', 'tw-lg', 'tw-xl', 'tw-2xl', 'tw-width', 'tw-container'])
})

test('two stylesheets made by the plugin declare the same order, whatever they contain', async () => {
  const a = await run(SHEET)
  const b = await run(`@layer theme, base, components, utilities; @layer utilities { .x { a: b; } @media (width >= 96rem) { .\\32xl\\:x { a: c; } } }`)
  const order = (css) => css.match(/@layer theme, base[^;]*;/)[0]
  assert.equal(order(a), order(b))
})

test('every breakpoint and kind lands where Tailwind would sort it', () => {
  const at = (name, params) => plugin.layerFor({ name, params })
  for (const [rem, layer] of [[40, 'tw-sm'], [48, 'tw-md'], [64, 'tw-lg'], [80, 'tw-xl'], [96, 'tw-2xl']]) {
    assert.equal(at('media', `(width >= ${rem}rem)`), layer)
    assert.equal(at('media', `(min-width: ${rem}rem)`), layer)
  }
  assert.equal(at('media', '(width >= 50rem)'), 'tw-width')
  assert.equal(at('media', '(width < 40rem)'), 'tw-width')
  assert.equal(at('media', '(max-width: 40rem)'), 'tw-width')
  assert.equal(at('media', '(hover: hover)'), 'tw-state')
  assert.equal(at('media', '(prefers-reduced-motion: reduce)'), 'tw-state')
  assert.equal(at('media', '(forced-colors: active)'), 'tw-state')
  assert.equal(at('media', 'print'), 'tw-state')
  assert.equal(at('supports', '(color: color-mix(in lab, red, red))'), 'tw-state')
  assert.equal(at('container', '(width >= 28rem)'), 'tw-container')
  assert.equal(at('container', 'sidebar (width >= 28rem)'), 'tw-container')
})

test('rules outside utilities and everything that is not a variant stay where they are', async () => {
  const out = await run(SHEET)
  assert.match(squash(out), /@layer properties \{ :root \{ --tw-x: 0; \} \}/)
  assert.match(squash(out), /@layer base \{ \* \{ margin: 0; \} \}/)
  assert.ok(out.trimEnd().endsWith('.career-enter { animation: x 1s; }'), 'the tool\'s own unlayered classes stay last and unlayered')
})

test('variants of one kind keep their order, and several utilities layers all get handled', async () => {
  const out = await run(`@layer theme, base, components, utilities;
@layer utilities { @media (width >= 64rem) { .one { a: 1; } } @media (width >= 64rem) { .two { a: 2; } } }
@layer utilities { @media (width >= 64rem) { .three { a: 3; } } }`)
  const lg = out.slice(out.indexOf('@layer tw-lg {'))
  assert.ok(lg.indexOf('.one') < lg.indexOf('.two') && lg.indexOf('.two') < lg.indexOf('.three'))
  assert.equal(layerNames(out).filter((n) => n === 'tw-lg').length, 1)
})

test('a sheet without variants is returned unchanged', async () => {
  const css = `@layer theme, base, components, utilities;\n@layer utilities { .a { b: c; } }\n`
  assert.equal(await run(css), css)
})

test('a sheet without an order statement gets one', async () => {
  const out = await run(`@layer utilities { .a { b: c; } @media (width >= 64rem) { .lg\\:a { b: d; } } }`)
  assert.ok(out.startsWith('@layer theme, base, components, utilities, tw-state'), out.slice(0, 80))
})

test('running it again changes nothing', async () => {
  const once = await run(SHEET)
  assert.equal(await run(once), once)
})

test('an @media written inside a rule (nested) is not touched, only top-level blocks of the utilities layer move', async () => {
  const out = await run(`@layer theme, base, components, utilities;
@layer utilities { .a { color: red; @media (width >= 64rem) { color: blue; } } }`)
  assert.doesNotMatch(out, /@layer tw-/)
  assert.match(squash(out), /\.a \{ color: red; @media \(width >= 64rem\) \{ color: blue; \} \}/)
})

test('only the layers that are used get a block (no empty layers)', async () => {
  const out = await run(`@layer theme, base, components, utilities; @layer utilities { @media (width >= 64rem) { .a { b: c; } } }`)
  assert.deepEqual(layerNames(out).filter((n) => n.startsWith('tw-')), ['tw-lg'])
  assert.doesNotMatch(out, /@layer tw-[a-z0-9]+\s*\{\s*\}/)
})

test('the existing order statement is replaced, not repeated', async () => {
  const out = await run(SHEET)
  const statements = out.split('\n').filter((l) => /^@layer [^{]*;\s*$/.test(l.trim()))
  assert.equal(statements.length, 1)
  assert.ok(statements[0].includes('tw-container'))
})
