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
  disabled?: boolean
  variant?: 'default' | 'danger'
}

const variantClasses: Record<NonNullable<IconButtonProps['variant']>, string> = {
  default: 'text-gray-500 hover:bg-gray-100 hover:text-gray-700',
  danger: 'text-red-600 hover:bg-red-50 hover:text-red-700',
}

export function IconButton({ icon, label, onClick, disabled, variant = 'default' }: IconButtonProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      aria-label={label}
      title={label}
      className={`inline-flex h-7 w-7 items-center justify-center rounded transition-colors disabled:cursor-not-allowed disabled:opacity-40 ${variantClasses[variant]}`}
    >
      {icon}
    </button>
  )
}
