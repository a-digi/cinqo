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
