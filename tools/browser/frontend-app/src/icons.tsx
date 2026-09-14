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
