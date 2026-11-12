import { Decor } from '../Decor/Decor'
import { gradient, type Accent } from '../../Components/JobsPage/jobsTheme'
import { Arrow } from './Arrow'
import { Reveal } from './Reveal'

// What a page with no entry shows instead of an empty list (plan 262, shared by plan 266): the first steps in one small picture, a button that opens the whole
// infographic, and the page's own actions (such as "New platform"). It is gone as soon as there is an entry (the page decides: infoEntryFor).
export function InfoTeaser({
  idPrefix,
  accent,
  title,
  lead,
  steps,
  openLabel,
  onOpen,
  actions,
}: {
  // The start of the heading's id ("platform": "platform-teaser-title").
  idPrefix: string
  accent: Accent
  title: string
  lead: string
  steps: { id: string; title: string; text: string }[]
  openLabel: string
  onOpen: () => void
  // The page's own buttons, after the one that opens the infographic.
  actions?: React.ReactNode
}) {
  return (
    <section
      aria-labelledby={`${idPrefix}-teaser-title`}
      className="relative overflow-hidden rounded-3xl border p-6"
      style={{ borderColor: accent.soft, background: `linear-gradient(160deg, #ffffff 40%, ${accent.soft} 100%)` }}
    >
      <Decor shape="ring" accent={accent} />
      <div className="relative">
        <h2 id={`${idPrefix}-teaser-title`} className="text-lg font-bold text-gray-900">
          {title}
        </h2>
        <p className="mt-1 max-w-xl text-sm text-gray-600">{lead}</p>
        <ol className="my-5 flex flex-col gap-2 md:flex-row md:items-stretch md:gap-1">
          {steps.map((s, i) => (
            <li key={s.id} className="flex flex-1 flex-col items-stretch md:flex-row md:items-center md:gap-1">
              <Reveal delay={i} className="flex-1">
                <div
                  className="flex h-full items-start gap-3 rounded-2xl border bg-white p-3 shadow-sm"
                  style={{ borderColor: accent.soft }}
                >
                  <span
                    className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-sm font-bold text-white"
                    style={{ background: gradient(accent) }}
                    aria-hidden="true"
                  >
                    {i + 1}
                  </span>
                  <div className="min-w-0">
                    <div className="text-sm font-semibold text-gray-900">{s.title}</div>
                    <div className="text-xs text-gray-500">{s.text}</div>
                  </div>
                </div>
              </Reveal>
              {i < steps.length - 1 && <Arrow accent={accent} className="mx-auto my-1 rotate-90 md:mx-0 md:my-0 md:rotate-0" />}
            </li>
          ))}
        </ol>
        <div className="flex flex-wrap items-center gap-3">
          <button
            type="button"
            onClick={onOpen}
            className="rounded-full px-4 py-2 text-sm font-semibold text-white shadow-sm transition-transform hover:-translate-y-0.5 motion-reduce:transition-none motion-reduce:hover:translate-y-0"
            style={{ background: gradient(accent) }}
          >
            {openLabel}
          </button>
          {actions}
        </div>
      </div>
    </section>
  )
}
