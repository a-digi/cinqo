import { gradient, type Accent } from '../../Components/JobsPage/jobsTheme'
import { Arrow } from './Arrow'

// The small, still area in a page's left column (plan 262, shared by plan 266): an icon and a title, one line, the first parts of the picture and a button that
// opens the whole infographic. Not a slider: nothing moves by itself.
export function InfoCard({
  idPrefix,
  accent,
  icon,
  title,
  text,
  parts,
  buttonLabel,
  onOpen,
}: {
  // The start of the heading's id ("platform": "platform-card-title").
  idPrefix: string
  accent: Accent
  icon: React.ReactNode
  title: string
  text: string
  parts: string[]
  buttonLabel: string
  onOpen: () => void
}) {
  return (
    <section
      aria-labelledby={`${idPrefix}-card-title`}
      className="mb-4 rounded-2xl border p-4"
      style={{ borderColor: accent.soft, background: accent.soft }}
    >
      <div className="flex items-center gap-2">
        <span
          className="flex h-8 w-8 shrink-0 items-center justify-center rounded-xl text-white"
          style={{ background: gradient(accent) }}
          aria-hidden="true"
        >
          {icon}
        </span>
        <h3 id={`${idPrefix}-card-title`} className="text-sm font-semibold text-gray-900">
          {title}
        </h3>
      </div>
      <p className="mt-2 text-xs leading-relaxed text-gray-600">{text}</p>
      <ol className="mt-3 flex items-center justify-between text-[11px] font-semibold text-gray-700">
        {parts.map((p, i) => (
          <li key={p} className="flex items-center gap-0.5">
            <span>{p}</span>
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
        {buttonLabel}
      </button>
    </section>
  )
}
