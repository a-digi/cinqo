// A small, icon-agnostic button — the caller supplies the icon, this
// component supplies the clickable affordance (hover state, disabled
// state, accessible name). Hand-rolled against Tailwind utility
// classes, no external UI/icon library, matching every other
// Shared/Components member's own convention. First built for the
// Tools admin page's row actions — see
// plan/ai/tools/step-13-tool-row-action-icons.md — but deliberately
// generic so any future page can reuse it with its own icon.
export interface IconButtonProps {
  icon: React.ReactNode
  label: string
  onClick: () => void
  // Optional escape hatch for a button that sits next to a focused
  // input (e.g. a Save/Cancel pair beside a rename <input>) — a plain
  // click would blur that input first, running its onBlur handler
  // before this button's own onClick. Passing
  // `(e) => e.preventDefault()` here stops that blur from happening at
  // all. See plan/ai/conversation/step-10-sidebar-action-icons.md.
  onMouseDown?: (e: React.MouseEvent) => void
  disabled?: boolean
  variant?: 'default' | 'danger'
  // A circular, white, shadowed button meant to sit on top of a
  // colored/media background (e.g. the Tools page's own card banner) —
  // same clickable affordance, different container shape/elevation
  // than the default flat-on-white icon button. First used by
  // ToolsListPage's card redesign.
  floating?: boolean
}

const variantClasses: Record<NonNullable<IconButtonProps['variant']>, string> = {
  default: 'text-gray-500 hover:bg-gray-100 hover:text-gray-700',
  danger: 'text-red-600 hover:bg-red-50 hover:text-red-700',
}

const floatingVariantClasses: Record<NonNullable<IconButtonProps['variant']>, string> = {
  default: 'bg-white text-gray-600 shadow hover:text-gray-900',
  danger: 'bg-white text-red-600 shadow hover:text-red-700',
}

export function IconButton({
  icon,
  label,
  onClick,
  onMouseDown,
  disabled,
  variant = 'default',
  floating = false,
}: IconButtonProps) {
  const shapeClasses = floating ? 'h-9 w-9 rounded-full' : 'h-7 w-7 rounded'
  const colorClasses = floating ? floatingVariantClasses[variant] : variantClasses[variant]
  return (
    <button
      type="button"
      onClick={onClick}
      onMouseDown={onMouseDown}
      disabled={disabled}
      aria-label={label}
      title={label}
      className={`inline-flex items-center justify-center transition-colors disabled:cursor-not-allowed disabled:opacity-40 ${shapeClasses} ${colorClasses}`}
    >
      {icon}
    </button>
  )
}
