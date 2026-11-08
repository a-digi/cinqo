import type { ProfileCompleteness } from '../../api'
import { bandLabel, missingItems } from './profileRules'

// Band colours for the bar only; the percentage and the band word are always printed too.
const BAND_COLOR: Record<ProfileCompleteness['band'], string> = {
  'just-started': '#dc2626',
  'getting-there': '#d97706',
  strong: '#2563eb',
  complete: '#16a34a',
}

// How complete one profile is: a bar with the percentage and the band word, and, when
// something is missing, what is missing (heaviest first) as plain text.
export function CompletenessMeter({ completeness }: { completeness: ProfileCompleteness }) {
  const { score, band, items } = completeness
  const missing = missingItems(items)
  return (
    <div className="mb-3">
      <div className="mb-1 flex items-baseline justify-between gap-2 text-xs">
        <span className="font-semibold text-gray-700">
          Profile completeness: {score}% <span className="font-normal text-gray-500">· {bandLabel(band)}</span>
        </span>
      </div>
      <div
        role="progressbar"
        aria-label="Profile completeness"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={score}
        aria-valuetext={`${score}%, ${bandLabel(band)}`}
        className="h-2 w-full overflow-hidden rounded-full bg-gray-200"
      >
        <div className="h-full rounded-full transition-all" style={{ width: `${score}%`, background: BAND_COLOR[band] }} />
      </div>
      {missing.length > 0 && (
        <p className="mt-1.5 text-xs text-gray-500">Missing: {missing.map((m) => `${m.label} (+${m.weight})`).join(', ')}</p>
      )}
    </div>
  )
}
