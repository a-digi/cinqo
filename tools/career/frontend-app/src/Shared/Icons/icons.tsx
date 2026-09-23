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

// CrawlInstructionsIcon — marks "Job list crawl instructions" (how to read the
// LISTING page, i.e. many rows at once): a document with several equal
// horizontal lines, each standing for one row of the list. Distinct
// from LogIcon's own similar document shape, which means "this run's
// own log," not "this link's own instructions."
export function CrawlInstructionsIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-5 w-5">
      <rect x="3.5" y="3" width="13" height="14" rx="1.2" strokeWidth="1.3" />
      <path d="M6.5 6.8h7M6.5 10h7M6.5 13.2h7" strokeWidth="1.3" strokeLinecap="round" />
    </svg>
  )
}

// JobDetailInstructionsIcon — marks "Job detail crawl instructions"
// (how to read ONE job's own detail page, not the listing). Previously
// shared CrawlInstructionsIcon's own plain-rectangle document base with
// a magnifying glass overlaid — too similar a silhouette to read as a
// genuinely different icon at a glance, especially once both sit
// side-by-side at a larger size. Redesigned as a single folded-corner
// page (a distinct outline, not just a decorated variant of the list
// icon's own rectangle) with two short lines standing for one focused
// block of detail text, rather than the list icon's three equal rows.
export function JobDetailInstructionsIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-5 w-5">
      <path d="M6 3h5l4 4v9a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1z" strokeWidth="1.3" strokeLinejoin="round" />
      <path d="M11 3v4h4" strokeWidth="1.3" strokeLinejoin="round" />
      <path d="M7 11.5h6M7 14h4" strokeWidth="1.3" strokeLinecap="round" />
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

// PDFIcon — the Import CV page's own dropzone centerpiece (a plain
// document-with-folded-corner glyph, no "PDF" text baked in since this
// is an SVG path, not a font glyph — the surrounding UI copy already
// says "PDF"). Deliberately larger than every other icon in this file
// (those are small inline action icons; this is a big, empty-state
// centerpiece), so it takes an explicit className rather than a
// hardcoded size.
export function PDFIcon({ className = 'h-16 w-16' }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" className={className}>
      <path d="M6 2.5h8.5L19 7v14a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V3.5a1 1 0 0 1 1-1z" strokeWidth="1.2" strokeLinejoin="round" />
      <path d="M14.5 2.5V7H19" strokeWidth="1.2" strokeLinejoin="round" />
      <path d="M8 13h8M8 16.2h8M8 9.8h4" strokeWidth="1.2" strokeLinecap="round" />
    </svg>
  )
}

// UploadIcon — an upward arrow into a tray, the standard upload glyph.
// Paired with PDFIcon as a small badge overlapping its corner on the
// Import CV dropzone. Same explicit-className convention as PDFIcon,
// for the same reason.
export function UploadIcon({ className = 'h-4 w-4' }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" className={className}>
      <path d="M12 15.5V4.5M8.2 8.3L12 4.5l3.8 3.8" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M4.5 15.5v3a2 2 0 0 0 2 2h11a2 2 0 0 0 2-2v-3" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

// EyeIcon — marks "view details" (a plain, read-only navigation
// action), distinct from every destructive/action-triggering icon in
// this file.
export function EyeIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-3.5 w-3.5">
      <path d="M2 10s2.8-5 8-5 8 5 8 5-2.8 5-8 5-8-5-8-5z" strokeWidth="1.3" strokeLinejoin="round" />
      <circle cx="10" cy="10" r="2.2" strokeWidth="1.3" />
    </svg>
  )
}

// ExternalLinkIcon — marks "open the original, external page in a new
// tab" — a box with an arrow escaping its own corner, the standard
// external-link glyph.
export function ExternalLinkIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-3.5 w-3.5">
      <path
        d="M8.5 4.5h-4a1 1 0 0 0-1 1v9a1 1 0 0 0 1 1h9a1 1 0 0 0 1-1v-4"
        strokeWidth="1.3"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path d="M9 3h5.5V8.5M14.5 3L8 9.5" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

// TrashIcon — marks a destructive delete action, replacing a plain
// text "Delete"/"Remove" button wherever this file's own icon-button
// convention applies.
export function TrashIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-3.5 w-3.5">
      <path d="M4 6h12M8 6V4.5a1 1 0 0 1 1-1h2a1 1 0 0 1 1 1V6" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M5.5 6l0.6 9.2a1 1 0 0 0 1 0.9h5.8a1 1 0 0 0 1-0.9L14.5 6" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M8.3 9v4M11.7 9v4" strokeWidth="1.2" strokeLinecap="round" />
    </svg>
  )
}

// MatchIcon — marks the "Job Match" action, a target/bullseye glyph
// distinct from every other icon in this file.
export function MatchIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-3.5 w-3.5">
      <circle cx="10" cy="10" r="7" strokeWidth="1.3" />
      <circle cx="10" cy="10" r="4" strokeWidth="1.3" />
      <circle cx="10" cy="10" r="1" fill="currentColor" stroke="none" />
    </svg>
  )
}

// RobotIcon (step 60) — marks "Check Progress - AI" (a
// generate-crawl-instructions conversation currently in flight),
// distinct from SparkleIcon's own "start this with AI" meaning.
export function RobotIcon({ className = 'h-3.5 w-3.5' }: { className?: string }) {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className={className}>
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

// DotsVerticalIcon — the standard "kebab menu" trigger glyph (three
// vertical dots), used by Shared/ActionMenu/ActionMenu.tsx.
export function DotsVerticalIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="currentColor" className="h-4 w-4">
      <circle cx="10" cy="4.5" r="1.4" />
      <circle cx="10" cy="10" r="1.4" />
      <circle cx="10" cy="15.5" r="1.4" />
    </svg>
  )
}
