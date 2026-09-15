// Local, self-contained icons — same reasoning as browser's own
// src/icons.tsx: this tool's build must not import from the main
// app's own component tree.
export function PlusIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-4 w-4">
      <path d="M10 5v10M5 10h10" strokeWidth="1.6" strokeLinecap="round" />
    </svg>
  )
}

export function XIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-3.5 w-3.5">
      <path d="M6 6l8 8M14 6l-8 8" strokeWidth="1.6" strokeLinecap="round" />
    </svg>
  )
}

export function PlayIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-3.5 w-3.5">
      <path d="M6 4.5l9 5.5-9 5.5v-11z" strokeWidth="1.4" strokeLinejoin="round" />
    </svg>
  )
}

// SparkleIcon (step 33) — marks the "generate/edit with AI" action,
// distinct from PlayIcon's own "run something" meaning.
export function SparkleIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-3.5 w-3.5">
      <path
        d="M10 3.5l1.2 3.3 3.3 1.2-3.3 1.2-1.2 3.3-1.2-3.3-3.3-1.2 3.3-1.2 1.2-3.3z"
        strokeWidth="1.2"
        strokeLinejoin="round"
      />
      <path d="M15.5 13l0.6 1.6 1.6 0.6-1.6 0.6-0.6 1.6-0.6-1.6-1.6-0.6 1.6-0.6 0.6-1.6z" strokeWidth="1" strokeLinejoin="round" />
    </svg>
  )
}

// LogIcon (step 38) — marks the "view this crawl run's own log" toggle,
// a plain lined-document glyph distinct from every other action icon
// on this page.
export function LogIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-3.5 w-3.5">
      <rect x="4" y="3" width="12" height="14" rx="1.2" strokeWidth="1.3" />
      <path d="M7 7h6M7 10h6M7 13h3.5" strokeWidth="1.3" strokeLinecap="round" />
    </svg>
  )
}
