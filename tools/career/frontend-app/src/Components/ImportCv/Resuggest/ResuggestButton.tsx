import { SparkleIcon } from '../../../Shared/Icons/icons'

// The small AI button of the Re-suggest feature (plan 268): on a section's heading, or (compact) on one row of a list. It only says what it is for; the dialog does the rest.
export function ResuggestButton({ ariaLabel, compact = false, onClick }: { ariaLabel: string; compact?: boolean; onClick: () => void }) {
  return (
    <button
      type="button"
      data-testid={compact ? 'resuggest-row-button' : 'resuggest-section-button'}
      aria-label={ariaLabel}
      title={compact ? 'Let the AI look at this one again' : 'Let the AI look at this section again'}
      onClick={onClick}
      className={`flex shrink-0 items-center gap-1.5 rounded-full border border-purple-200 bg-white font-semibold text-purple-700 shadow-sm transition-all hover:-translate-y-0.5 hover:bg-purple-50 hover:shadow motion-reduce:transition-none motion-reduce:hover:translate-y-0 ${
        compact ? 'px-2.5 py-1 text-xs' : 'px-3 py-1.5 text-xs'
      }`}
    >
      <SparkleIcon />
      {compact ? 'Re-suggest' : 'Re-suggest'}
    </button>
  )
}
