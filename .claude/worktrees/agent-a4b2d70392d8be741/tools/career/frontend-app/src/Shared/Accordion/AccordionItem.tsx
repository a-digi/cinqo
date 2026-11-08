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
}

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
export function AccordionItem({ isOpen, onToggle, header, children, className, style, overlay }: AccordionItemProps) {
  function handleHeaderKeyDown(e: React.KeyboardEvent<HTMLDivElement>) {
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
      className={`relative rounded-xl border border-gray-200 bg-white transition-all hover:border-gray-300 hover:shadow-md ${overlay ? 'min-h-[280px] overflow-hidden' : ''} ${className ?? ''}`}
    >
      <div
        role="button"
        tabIndex={0}
        onClick={onToggle}
        onKeyDown={handleHeaderKeyDown}
        aria-expanded={isOpen}
        className="flex w-full cursor-pointer items-center justify-between gap-3 rounded-xl px-4 py-3 text-left"
      >
        <div className="min-w-0 flex-1">{header}</div>
        <span className={`shrink-0 text-gray-400 transition-transform duration-200 ${isOpen ? 'rotate-180' : ''}`}>
          <ChevronDownIcon />
        </span>
      </div>
      <div className="accordion-body" data-open={isOpen}>
        <div>
          <div className="px-4 pb-4">{children}</div>
        </div>
      </div>
      {overlay && (
        <div className="absolute inset-0 z-10 flex flex-col rounded-xl bg-white p-4 [animation:backdrop-in_150ms_ease-out]">{overlay}</div>
      )}
    </div>
  )
}
