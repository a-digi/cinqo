// A small rounded label — consolidates three near-identical inline
// implementations that had accumulated across the admin pages
// (ToolsListPage's Enabled cell and its own local StatusBadge,
// ScopesPage's own local Badge), all rendering the exact same
// `inline-flex rounded-full px-2 py-0.5 text-xs font-medium` shape
// with only the color pair differing. Hand-rolled against Tailwind
// utility classes, no external UI/badge library, matching IconButton's
// own variant-prop convention. Purely presentational — no API calls,
// no context reads.
export interface PillProps {
  children: React.ReactNode
  variant?: 'gray' | 'green' | 'amber' | 'red'
  // Native browser tooltip — used by the Tools page's own "+N more"
  // overflow pill to list the remaining names without a custom
  // tooltip component.
  title?: string
  // Bordered/neutral treatment instead of a filled color — for a
  // plain fact chip (e.g. "backend_only", "6 functions") that isn't a
  // status/semantic indicator. Ignores `variant` when set, since an
  // outline chip is always neutral. First used by the Tools page's own
  // card layout's stat-chip row.
  outline?: boolean
}

const variantClasses: Record<NonNullable<PillProps['variant']>, string> = {
  gray: 'bg-gray-100 text-gray-600',
  green: 'bg-green-100 text-green-700',
  amber: 'bg-amber-100 text-amber-700',
  red: 'bg-red-100 text-red-700',
}

const outlineClasses = 'border border-gray-200 bg-white text-gray-700'

export function Pill({ children, variant = 'gray', title, outline = false }: PillProps) {
  return (
    <span
      title={title}
      className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${outline ? outlineClasses : variantClasses[variant]}`}
    >
      {children}
    </span>
  )
}
