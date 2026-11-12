import { GlobeIcon } from '../../../Shared/Icons/icons'
import { ACCENTS, gradient } from '../../JobsPage/jobsTheme'
import { Arrow } from './diagrams/diagrams'
import { CARD_BUTTON, CARD_TEXT, CARD_TITLE, HERO_NODES } from './platformInfoData'

// The small, still area in the left column of the Platforms page (plan 262): a title, one line, the first three parts of the picture and a button that
// opens the whole infographic. Not a slider: nothing moves by itself.
export function PlatformInfoCard({ onOpen }: { onOpen: () => void }) {
  const accent = ACCENTS[0]
  const parts = HERO_NODES.slice(0, 3)
  return (
    <section
      aria-labelledby="platform-card-title"
      className="mb-4 rounded-2xl border p-4"
      style={{ borderColor: accent.soft, background: accent.soft }}
    >
      <div className="flex items-center gap-2">
        <span
          className="flex h-8 w-8 shrink-0 items-center justify-center rounded-xl text-white"
          style={{ background: gradient(accent) }}
          aria-hidden="true"
        >
          <GlobeIcon className="h-4 w-4" />
        </span>
        <h3 id="platform-card-title" className="text-sm font-semibold text-gray-900">
          {CARD_TITLE}
        </h3>
      </div>
      <p className="mt-2 text-xs leading-relaxed text-gray-600">{CARD_TEXT}</p>
      <ol className="mt-3 flex items-center justify-between text-[11px] font-semibold text-gray-700">
        {parts.map((p, i) => (
          <li key={p.id} className="flex items-center gap-0.5">
            <span>{p.label}</span>
            {i < parts.length - 1 && <Arrow accent={accent} className="scale-75" />}
          </li>
        ))}
      </ol>
      <button
        type="button"
        onClick={onOpen}
        className="mt-3 w-full rounded-full border bg-white px-3 py-1.5 text-xs font-semibold hover:bg-blue-50"
        style={{ borderColor: accent.from, color: accent.text }}
      >
        {CARD_BUTTON}
      </button>
    </section>
  )
}
