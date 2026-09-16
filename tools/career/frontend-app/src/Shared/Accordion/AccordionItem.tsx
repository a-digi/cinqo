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
export function AccordionItem({ isOpen, onToggle, header, children, className, style }: AccordionItemProps) {
  function handleHeaderKeyDown(e: React.KeyboardEvent<HTMLDivElement>) {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      onToggle()
    }
  }

  return (
    <div
      style={style}
      className={`rounded-xl border border-gray-200 bg-white transition-all hover:border-gray-300 hover:shadow-md ${className ?? ''}`}
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
    </div>
  )
}
