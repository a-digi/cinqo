import { SparkleIcon } from '../../../Shared/Icons/icons'

// The small AI button of the Re-suggest feature (plan 268): on a section's heading, or (compact) on one row of a list. It only says what it is for; the dialog does the rest.
export function ResuggestButton({
  ariaLabel,
  compact = false,
  label = 'Re-suggest',
  onClick,
}: {
  ariaLabel: string
  compact?: boolean
  // The words on the button: a section's button says how many rows it covers ("Re-suggest all 4").
  label?: string
  onClick: () => void
}) {
  return (
    <button
      type="button"
      data-testid={compact ? 'resuggest-row-button' : 'resuggest-section-button'}
      aria-label={ariaLabel}
      title={compact ? 'Let the AI look at this one again' : 'Let the AI look at the whole section again'}
      onClick={onClick}
      className={`flex shrink-0 items-center gap-1.5 rounded-full border font-semibold shadow-sm transition-all hover:-translate-y-0.5 hover:shadow motion-reduce:transition-none motion-reduce:hover:translate-y-0 ${
        compact
          ? 'border-purple-200 bg-white px-2.5 py-1 text-xs text-purple-700 hover:bg-purple-50'
          : 'border-purple-600 bg-purple-600 px-3.5 py-1.5 text-xs text-white hover:bg-purple-700'
      }`}
    >
      <SparkleIcon />
      {label}
    </button>
  )
}
