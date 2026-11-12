import { Decor } from '../../../Shared/Decor/Decor'
import { PlusIcon } from '../../../Shared/Icons/icons'
import { ACCENTS, gradient } from '../../JobsPage/jobsTheme'
import { Arrow } from './diagrams/diagrams'
import { INFO_BUTTON, TEASER_LEAD, TEASER_STEPS, TEASER_TITLE } from './platformInfoData'
import { Reveal } from './Reveal'

// What a page with no platform shows instead of an empty list (plan 262): the first three steps in one small picture and a button that opens the whole
// infographic. It is gone as soon as there is a platform (the page decides: infoEntryFor).
export function PlatformInfoTeaser({ onOpen, onNew }: { onOpen: () => void; onNew: () => void }) {
  const accent = ACCENTS[0]
  return (
    <section
      aria-labelledby="platform-teaser-title"
      className="relative overflow-hidden rounded-3xl border p-6"
      style={{ borderColor: accent.soft, background: `linear-gradient(160deg, #ffffff 40%, ${accent.soft} 100%)` }}
    >
      <Decor shape="ring" accent={accent} />
      <div className="relative">
        <h2 id="platform-teaser-title" className="text-lg font-bold text-gray-900">
          {TEASER_TITLE}
        </h2>
        <p className="mt-1 max-w-xl text-sm text-gray-600">{TEASER_LEAD}</p>
        <ol className="my-5 flex flex-col gap-2 md:flex-row md:items-stretch md:gap-1">
          {TEASER_STEPS.map((s, i) => (
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
              {i < TEASER_STEPS.length - 1 && <Arrow accent={accent} className="mx-auto my-1 rotate-90 md:mx-0 md:my-0 md:rotate-0" />}
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
            {INFO_BUTTON}
          </button>
          <button
            type="button"
            onClick={onNew}
            className="flex items-center gap-1.5 rounded-full border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-800 hover:bg-gray-50"
          >
            <PlusIcon />
            New platform
          </button>
        </div>
      </div>
    </section>
  )
}
