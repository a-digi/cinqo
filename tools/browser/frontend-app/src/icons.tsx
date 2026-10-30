// The exact SVG this tool's own vanilla-JS predecessor (step 14)
// copied from frontend/src/Shared/Components/IconButton/icons.tsx —
// kept here as a literal, local copy (not an import) since this
// tool's own build must stay fully self-contained, independent of
// the main app's own source tree. See
// plan/ai/tools/browser/step-15-react-tailwind-frontend-rewrite.md.
export function PlusIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-4 w-4">
      <path d="M10 5v10M5 10h10" strokeWidth="1.6" strokeLinecap="round" />
    </svg>
  )
}

// Page-header glyphs, drawn the way the Career tool's header icons are (a 24px grid,
// round strokes, sized by the badge around them).
function Glyph({ children, className = 'h-6 w-6' }: { children: React.ReactNode; className?: string }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      className={className}
    >
      {children}
    </svg>
  )
}

// Browser & Engine.
export function ShieldIcon({ className }: { className?: string }) {
  return (
    <Glyph className={className}>
      <path d="M12 3.5 5 6v5.5c0 4.2 2.9 7.4 7 9 4.1-1.6 7-4.8 7-9V6l-7-2.5z" />
      <path d="m9 12 2.2 2.2L15.2 10" />
    </Glyph>
  )
}

// Login.
export function KeyIcon({ className }: { className?: string }) {
  return (
    <Glyph className={className}>
      <circle cx="8" cy="15" r="3.6" />
      <path d="m10.7 12.4 8.3-8.3M15.5 7.6l2.4 2.4M13 10.1l1.8 1.8" />
    </Glyph>
  )
}

// Domains.
export function GlobeIcon({ className }: { className?: string }) {
  return (
    <Glyph className={className}>
      <circle cx="12" cy="12" r="9" />
      <path d="M3 12h18M12 3c2.6 2.7 3.8 5.7 3.8 9s-1.2 6.3-3.8 9c-2.6-2.7-3.8-5.7-3.8-9S9.4 5.7 12 3z" />
    </Glyph>
  )
}

// Debug.
export function BugIcon({ className }: { className?: string }) {
  return (
    <Glyph className={className}>
      <path d="M9 7.5a3 3 0 0 1 6 0M8 10.5h8v4.5a4 4 0 0 1-8 0v-4.5z" />
      <path d="M12 10.5V19M4 11l3 1M20 11l-3 1M4 17l3-1.5M20 17l-3-1.5M6 7l2 1.5M18 7l-2 1.5" />
    </Glyph>
  )
}

// Crawl Logs.
export function ListIcon({ className }: { className?: string }) {
  return (
    <Glyph className={className}>
      <path d="M8.5 6.5H20M8.5 12H20M8.5 17.5H20" />
      <circle cx="4.5" cy="6.5" r="1" />
      <circle cx="4.5" cy="12" r="1" />
      <circle cx="4.5" cy="17.5" r="1" />
    </Glyph>
  )
}

// The Chrome tile on Browser & Engine.
export function ChromeIcon({ className }: { className?: string }) {
  return (
    <Glyph className={className}>
      <circle cx="12" cy="12" r="9" />
      <circle cx="12" cy="12" r="3.4" />
      <path d="M12 8.6h8.3M9.1 13.7 5 20.1M14.9 13.7 11 20.4" />
    </Glyph>
  )
}

// The vault card on the Login page.
export function LockIcon({ className }: { className?: string }) {
  return (
    <Glyph className={className}>
      <rect x="5" y="10.5" width="14" height="9.5" rx="2" />
      <path d="M8.5 10.5V8a3.5 3.5 0 0 1 7 0v2.5" />
      <path d="M12 14.5v2" />
    </Glyph>
  )
}

// The Debug page's logging gauge: page HTML and challenge dumps (the crawl log layer uses ListIcon).
export function CodeIcon({ className }: { className?: string }) {
  return (
    <Glyph className={className}>
      <path d="m8.5 8-4 4 4 4M15.5 8l4 4-4 4M13.5 6l-3 12" />
    </Glyph>
  )
}

export function ChallengeIcon({ className }: { className?: string }) {
  return (
    <Glyph className={className}>
      <path d="M12 3.5 5 6v5.5c0 4.2 2.9 7.4 7 9 4.1-1.6 7-4.8 7-9V6l-7-2.5z" />
      <path d="M12 8.5v4M12 15.6v.1" />
    </Glyph>
  )
}
