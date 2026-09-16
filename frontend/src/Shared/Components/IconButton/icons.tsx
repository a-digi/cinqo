// Small, hand-rolled icon glyphs for use with IconButton — same
// viewBox/stroke="currentColor" convention already established by
// InfoIcon (MessageThread.tsx) and PanelIcon (ConversationPage.tsx).
// Colocated here rather than redrawn per page, so the next feature
// needing one of these reuses it instead of a new copy.

export function PowerIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-4 w-4">
      <path d="M10 4v5.5" strokeWidth="1.4" strokeLinecap="round" />
      <path d="M6.5 5.5a5.5 5.5 0 1 0 7 0" strokeWidth="1.4" strokeLinecap="round" />
    </svg>
  )
}

export function TrashIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-4 w-4">
      <path d="M4.5 6h11" strokeWidth="1.4" strokeLinecap="round" />
      <path d="M8 6V4.5h4V6" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M6 6l.5 9.5a1 1 0 0 0 1 .95h5a1 1 0 0 0 1-.95L14 6" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M8.5 8.5v5M11.5 8.5v5" strokeWidth="1.4" strokeLinecap="round" />
    </svg>
  )
}

export function PencilIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-4 w-4">
      <path
        d="M13.5 4.5l2 2-8.25 8.25H5v-2.25L13.25 4.5a1 1 0 0 1 1.42 0z"
        strokeWidth="1.4"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path d="M11.75 6.25l2 2" strokeWidth="1.4" strokeLinecap="round" />
    </svg>
  )
}

export function CheckIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-4 w-4">
      <path d="M4.5 10.5l3.5 3.5 7-8" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

export function XIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-4 w-4">
      <path d="M5.5 5.5l9 9M14.5 5.5l-9 9" strokeWidth="1.6" strokeLinecap="round" />
    </svg>
  )
}

export function PlusIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-4 w-4">
      <path d="M10 5v10M5 10h10" strokeWidth="1.6" strokeLinecap="round" />
    </svg>
  )
}

// LogsIcon (step 36) — a plain lined-document glyph marking "view this
// conversation's own AI trace logs," distinct from every other row
// action (Rename/Delete) in ConversationSidebar.tsx.
export function LogsIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-4 w-4">
      <rect x="4.5" y="3" width="11" height="14" rx="1.2" strokeWidth="1.3" />
      <path d="M7 7h6M7 10h6M7 13h3.5" strokeWidth="1.3" strokeLinecap="round" />
    </svg>
  )
}
