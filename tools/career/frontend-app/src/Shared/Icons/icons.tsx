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

// StopIcon (step 63) — marks a real, backend-effecting "stop this
// crawl" action, distinct from PlayIcon's own "start/resume" meaning.
// A filled square, the universal stop glyph — the only icon in this
// file using fill instead of stroke, deliberately, so it reads
// distinctly at a glance from every other action icon here.
export function StopIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="currentColor" className="h-3.5 w-3.5">
      <rect x="5" y="5" width="10" height="10" rx="1.5" />
    </svg>
  )
}

// SparkleIcon (step 33) — marks the "generate/edit with AI" action,
// distinct from PlayIcon's own "run something" meaning.
export function SparkleIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-3.5 w-3.5">
      <path d="M10 3.5l1.2 3.3 3.3 1.2-3.3 1.2-1.2 3.3-1.2-3.3-3.3-1.2 3.3-1.2 1.2-3.3z" strokeWidth="1.2" strokeLinejoin="round" />
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

// AlertIcon (step 40) — marks the one running-phase state that needs
// the user's own action (awaiting_human_challenge), distinct from
// every other, passive status glyph on this page.
export function AlertIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-3.5 w-3.5">
      <path d="M10 3l8 14H2l8-14z" strokeWidth="1.3" strokeLinejoin="round" />
      <path d="M10 8.5v3.2" strokeWidth="1.3" strokeLinecap="round" />
      <circle cx="10" cy="14" r="0.6" fill="currentColor" stroke="none" />
    </svg>
  )
}

// FilterIcon (step 55) — a plain funnel glyph marking the Jobs page's
// own "Filters" toggle button.
export function FilterIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-3.5 w-3.5">
      <path d="M3 4h14l-5.5 6.5v4.5l-3 1.5v-6z" strokeWidth="1.3" strokeLinejoin="round" />
    </svg>
  )
}

// ChevronDownIcon (step 58) — the Accordion's own expand/collapse
// indicator, rotated 180deg by the caller when open rather than
// swapped for a separate "up" glyph.
export function ChevronDownIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-4 w-4">
      <path d="M5 7.5l5 5 5-5" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

// RobotIcon (step 60) — marks "Check Progress - AI" (a
// generate-crawl-instructions conversation currently in flight),
// distinct from SparkleIcon's own "start this with AI" meaning.
export function RobotIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-3.5 w-3.5">
      <path d="M10 3v2" strokeWidth="1.3" strokeLinecap="round" />
      <circle cx="10" cy="4.2" r="0.7" fill="currentColor" stroke="none" />
      <rect x="4.5" y="6" width="11" height="9" rx="2" strokeWidth="1.3" />
      <circle cx="7.5" cy="10.2" r="0.9" fill="currentColor" stroke="none" />
      <circle cx="12.5" cy="10.2" r="0.9" fill="currentColor" stroke="none" />
      <path d="M7.5 13h5" strokeWidth="1.2" strokeLinecap="round" />
      <path d="M4.5 9.5h-1.2M16.7 9.5h-1.2" strokeWidth="1.2" strokeLinecap="round" />
    </svg>
  )
}
