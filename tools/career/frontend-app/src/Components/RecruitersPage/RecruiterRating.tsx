import { Stars } from '../../Shared/Reviews/Stars'
import { REVIEW_TYPE_RECRUITER } from '../../Shared/Reviews/reviewTypes'

// The rating line of a recruiter's row (plan 271), the same as a company's: the small stars (average and count) and a Rate button that
// asks the page to open the rating panel. `starsVersion` goes up when the panel closes, so the stars ask again (the rating may have changed).
export function RecruiterRating({ recruiterId, starsVersion, onRate }: { recruiterId: string; starsVersion: number; onRate: () => void }) {
  return (
    <div className="mt-1 flex items-center gap-2">
      <Stars key={`${recruiterId}-${String(starsVersion)}`} entityType={REVIEW_TYPE_RECRUITER} entityId={recruiterId} />
      <button
        type="button"
        onClick={onRate}
        className="rounded-md border border-amber-200 bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-800 hover:bg-amber-100"
      >
        Rate
      </button>
    </div>
  )
}
