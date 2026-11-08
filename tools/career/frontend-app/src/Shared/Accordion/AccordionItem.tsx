import { ChevronDownIcon } from '../Icons/icons'

interface AccordionItemProps {
  isOpen: boolean
  onToggle: () => void
  header: React.ReactNode
  children: React.ReactNode
  className?: string
  // Passed straight through to the outer card — the mount-cascade
  // fade-in's own per-item animationDelay (PortalsPage) has no
  // Tailwind utility equivalent, since it's a dynamic, per-index value.
  style?: React.CSSProperties
  // When non-null, rendered as an opaque white layer covering the
  // entire card (header included) — header/children stay mounted
  // underneath (no state loss) but are fully hidden. AccordionItem has
  // no idea what overlay contains or why it's shown — purely a
  // controlled "cover this card" primitive, same as isOpen/onToggle.
  // See plan/ai/tools/career/step-59-portals-add-link-container-overlay.md.
  overlay?: React.ReactNode
  // variant (step 89) — 'card' (the default, and the only look before
  // this prop existed) is a free-standing card; 'nested' is the compact
  // look for an accordion that sits inside another card (a portal
  // link inside its portal): smaller radius and padding, a tinted
  // background, no hover shadow; 'plain' is a bare labeled, collapsible
  // group with no border, background or padding (a section inside a
  // panel, e.g. CrawlPanel's "Crawl instructions"). Purely visual —
  // behavior is identical.
  // See plan/ai/tools/career/step-89-portal-link-accordion.md.
  variant?: 'card' | 'nested' | 'plain' | 'portal'
}

// Per-variant class sets. 'card' is byte-for-byte what AccordionItem
// rendered before variants existed.
const VARIANT_CLASSES = {
  card: {
    outer: 'rounded-xl border border-gray-200 bg-white transition-all hover:border-gray-300 hover:shadow-md',
    header: 'rounded-xl px-4 py-3',
    body: 'px-4 pb-4',
    overlay: 'rounded-xl',
  },
  nested: {
    outer: 'rounded-2xl border border-blue-100 bg-blue-50/40 transition-colors hover:border-blue-200',
    header: 'rounded-2xl px-3.5 py-2.5',
    body: 'px-3.5 pb-3.5',
    overlay: 'rounded-2xl',
  },
  // A portal: a big rounded card like the Jobs page's cards. Its tint (border colour and
  // background gradient) comes from the caller's style, since it depends on the portal.
  portal: {
    outer: 'rounded-3xl border shadow-sm transition-shadow hover:shadow-md',
    header: 'rounded-3xl px-4 py-3.5',
    body: 'px-4 pb-4',
    overlay: 'rounded-3xl',
  },
  plain: {
    outer: '',
    header: 'rounded-md',
    body: '',
    overlay: 'rounded-md',
  },
} as const

// Shared accordion item — controlled (isOpen/onToggle only), no
// internal open state, no "only one open at a time" group logic (see
// the design doc's own flagged open question). The body's
// expand/collapse is a CSS grid row-size transition (.accordion-body
// in index.css), not a fixed max-height guess, so it animates
// correctly regardless of the body's actual rendered height.
//
// The header row is a div[role="button"], not a real <button> — a
// header can itself contain interactive children (PortalsPage's own
// Edit/Delete buttons and its inline-rename <input>), and nesting
// interactive elements inside a native <button> is invalid HTML. Those
// nested controls stop click propagation themselves so clicking them
// doesn't also toggle the accordion. See
// plan/ai/tools/career/step-58-portals-page-modal-accordion-redesign.md.
export function AccordionItem({ isOpen, onToggle, header, children, className, style, overlay, variant = 'card' }: AccordionItemProps) {
  const v = VARIANT_CLASSES[variant]
  function handleHeaderKeyDown(e: React.KeyboardEvent<HTMLDivElement>) {
    // Only the header row's own key presses toggle it. A key pressed in
    // an input or button INSIDE the header (inline rename, Edit/Remove)
    // bubbles up here too, and must neither toggle the accordion nor
    // have its default (typing a space, activating a button) prevented.
    if (e.target !== e.currentTarget) return
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      onToggle()
    }
  }

  return (
    <div
      style={style}
      // min-h only while overlay is active: the overlay is
      // absolute/inset-0, so it's exactly as tall as this card's own
      // normal-flow content (header + body) — a portal with no links
      // yet has a very short body, shorter than the add-link form's
      // own content, so without a floor here the overlay clips its
      // own bottom (Create/Cancel buttons cut off). Not applied
      // unconditionally so a normal, overlay-less card keeps sizing to
      // its real content. See
      // plan/ai/tools/career/step-59-portals-add-link-container-overlay.md.
      className={`relative ${v.outer} ${overlay ? 'min-h-[280px] overflow-hidden' : ''} ${className ?? ''}`}
    >
      <div
        role="button"
        tabIndex={0}
        onClick={onToggle}
        onKeyDown={handleHeaderKeyDown}
        aria-expanded={isOpen}
        className={`flex w-full cursor-pointer items-center justify-between gap-3 text-left ${v.header}`}
      >
        <div className="min-w-0 flex-1">{header}</div>
        <span className={`shrink-0 text-gray-400 transition-transform duration-200 ${isOpen ? 'rotate-180' : ''}`}>
          <ChevronDownIcon />
        </span>
      </div>
      <div className="accordion-body" data-open={isOpen}>
        <div>
          <div className={v.body}>{children}</div>
        </div>
      </div>
      {overlay && (
        <div className={`absolute inset-0 z-10 flex flex-col bg-white p-4 [animation:backdrop-in_150ms_ease-out] ${v.overlay}`}>
          {overlay}
        </div>
      )}
    </div>
  )
}
