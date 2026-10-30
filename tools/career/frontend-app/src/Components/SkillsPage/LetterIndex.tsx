import { PAGE_ACCENT } from '../JobsPage/jobsTheme'
import type { LetterCount } from './skillRules'

// The Skills page's overview: a strip of letters above the skills. A letter shows how
// many skills start with it (dim when none); clicking one shows only those skills,
// clicking it again, or "All", shows everything.
export function LetterIndex({
  index,
  active,
  total,
  onPick,
}: {
  index: LetterCount[]
  active: string | null
  total: number
  onPick: (letter: string | null) => void
}) {
  return (
    <nav aria-label="Skills by first letter" className="career-enter career-delay-1 mb-3 flex flex-wrap items-center gap-1.5">
      <button
        type="button"
        aria-pressed={active === null}
        onClick={() => {
          onPick(null)
        }}
        className="rounded-full px-3 py-1 text-xs font-semibold"
        style={active === null ? { background: PAGE_ACCENT.to, color: '#fff' } : { background: PAGE_ACCENT.soft, color: PAGE_ACCENT.text }}
      >
        All <span className="opacity-70">{total}</span>
      </button>
      {index.map(({ letter, count }) => {
        const isActive = active === letter
        return (
          <button
            key={letter}
            type="button"
            disabled={count === 0}
            aria-pressed={isActive}
            onClick={() => {
              onPick(isActive ? null : letter)
            }}
            className={`flex h-8 min-w-8 items-center justify-center gap-0.5 rounded-lg px-1.5 text-xs font-semibold transition-colors ${
              count === 0 ? 'cursor-default text-gray-300' : isActive ? 'text-white' : 'bg-white text-gray-700 hover:bg-blue-50'
            }`}
            style={isActive ? { background: PAGE_ACCENT.to } : count > 0 ? { border: `1px solid ${PAGE_ACCENT.soft}` } : undefined}
          >
            {letter}
            {count > 0 && <sup className={`text-[10px] font-medium ${isActive ? 'text-white/80' : 'text-gray-400'}`}>{count}</sup>}
          </button>
        )
      })}
    </nav>
  )
}
