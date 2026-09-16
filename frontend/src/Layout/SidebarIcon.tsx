import { useMemo } from 'react'

// Renders a menu entry's own `icon` — raw SVG markup that may come
// from a tool's own (untrusted) bundle, not just first-party code —
// through a small, explicit allowlist sanitizer rather than a
// general-purpose library. The surface actually needed here is tiny
// and fixed (basic shape-drawing primitives for a 16x16 glyph), so a
// hand-rolled allowlist is easier to fully audit than correctly
// configuring a general sanitizer's own SVG profile, and adds no new
// dependency — matching toolBridge.ts's own "dependency-free"
// philosophy. See
// plan/ai/frontend/frontend/step-13-sidebar-icons-and-calculated-indentation.md.

// A payload longer than this is rejected before it's even parsed —
// icons.tsx's own existing glyphs are all well under 500 characters as
// raw markup, so this is generous headroom, not a tight fit.
const MAX_MENU_ICON_SVG_LENGTH = 2000

const ALLOWED_TAGS = new Set(['svg', 'path', 'g', 'circle', 'rect', 'line', 'polyline', 'polygon', 'ellipse', 'defs'])

const ALLOWED_ATTRIBUTES = new Set([
  'd',
  'viewBox',
  'fill',
  'stroke',
  'stroke-width',
  'stroke-linecap',
  'stroke-linejoin',
  'cx',
  'cy',
  'r',
  'rx',
  'ry',
  'x',
  'y',
  'x1',
  'y1',
  'x2',
  'y2',
  'points',
  'width',
  'height',
])

const SVG_NS = 'http://www.w3.org/2000/svg'

// Rebuilds a *new* tree from only allowlisted elements/attributes —
// never trusts the parsed input tree directly, so anything not
// explicitly kept (script, foreignObject, use, image, a, style
// attributes, on* handlers, href/xlink:href, ...) simply isn't copied
// over, rather than being individually blocked by a denylist (a
// denylist only ever covers vectors someone thought to list).
function sanitize(raw: string): SVGSVGElement | null {
  if (raw.length === 0 || raw.length > MAX_MENU_ICON_SVG_LENGTH) return null

  let parsed: Document
  try {
    parsed = new DOMParser().parseFromString(raw, 'image/svg+xml')
  } catch {
    return null
  }
  if (parsed.querySelector('parsererror')) return null

  const sourceRoot = parsed.documentElement
  if (sourceRoot.tagName.toLowerCase() !== 'svg') return null

  function cloneAllowed(node: Element): Element | null {
    const tag = node.tagName.toLowerCase()
    if (!ALLOWED_TAGS.has(tag)) return null

    const clone = document.createElementNS(SVG_NS, tag)
    for (const attr of Array.from(node.attributes)) {
      if (ALLOWED_ATTRIBUTES.has(attr.name)) {
        clone.setAttribute(attr.name, attr.value)
      }
    }
    for (const child of Array.from(node.children)) {
      const clonedChild = cloneAllowed(child)
      if (clonedChild) clone.appendChild(clonedChild)
    }
    return clone
  }

  const sanitizedRoot = cloneAllowed(sourceRoot) as SVGSVGElement | null
  if (!sanitizedRoot) return null

  // The root element's own size is never taken from the source —
  // fixed here regardless of whatever width/height it carried, so a
  // plugin can't ship an oversized icon that breaks row layout.
  sanitizedRoot.removeAttribute('width')
  sanitizedRoot.removeAttribute('height')
  if (!sanitizedRoot.getAttribute('fill')) sanitizedRoot.setAttribute('fill', 'none')
  if (!sanitizedRoot.getAttribute('stroke')) sanitizedRoot.setAttribute('stroke', 'currentColor')
  sanitizedRoot.setAttribute('class', 'h-4 w-4 shrink-0')

  return sanitizedRoot
}

export function SidebarIcon({ svg }: { svg: string }) {
  const markup = useMemo(() => {
    const sanitized = sanitize(svg)
    return sanitized ? new XMLSerializer().serializeToString(sanitized) : null
  }, [svg])

  // Always reserve the same h-4 w-4 slot, even when sanitization
  // fails outright (unparseable input, or a root that isn't <svg>) —
  // an entry with a malformed icon should still line up with its
  // siblings the same way an entry with no icon at all does, not
  // silently lose its own indentation width. Caught live: an
  // intentionally-garbage test payload rendered with zero reserved
  // space before this fix. See this component's own step-13 design
  // doc for the "reserve the icon slot uniformly" decision.
  return <span className="inline-flex h-4 w-4 shrink-0" dangerouslySetInnerHTML={markup ? { __html: markup } : undefined} />
}
