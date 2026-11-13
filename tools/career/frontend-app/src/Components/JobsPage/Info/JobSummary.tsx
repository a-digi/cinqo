import { AppLink } from '../../../Shared/AppLink/AppLink'
import { PLATFORMS_PATH } from '../../../paths'
import { ShowcasePill } from '../../../Shared/Infographic/showcase/ShowcasePill'
import { SHOWCASE } from '../../../Shared/Infographic/showcase/showcaseTokens'
import { FIRST_STEPS, GUIDE_LINK, TEASER_BUTTONS, TEASER_LEAD, TEASER_NEXT, TEASER_TITLE } from './jobInfoData'
import { journeyState, summaryNext, type JobFacts } from './jobInfoRules'

// What the Jobs page is when there is no job at all (plan 269, the user's rule of 2026-11-13): not an empty table under statistics, but only this summary: why the list is empty (jobs come from
// the links of the platforms, or from you), the next thing to do, the four first steps as dots, and the buttons: the one that opens the infographic first. It is gone the moment the first job arrives
// (the page decides: jobsPageView).
export function JobSummary({ facts, onOpen, onAddJob }: { facts: JobFacts; onOpen: () => void; onAddJob: () => void }) {
  const next = summaryNext(facts.platforms)
  const steps = journeyState(facts)
  return (
    <section
      aria-labelledby="job-summary-title"
      data-testid="job-summary"
      className="relative overflow-hidden rounded-[2rem] px-6 py-14 text-center md:px-12 md:py-20"
      style={{ background: SHOWCASE.black, color: SHOWCASE.white, fontFamily: SHOWCASE.font }}
    >
      <div
        aria-hidden="true"
        className="pointer-events-none absolute left-1/2 top-0 h-80 w-[44rem] max-w-none -translate-x-1/2 rounded-full opacity-35 blur-3xl"
        style={{ background: `radial-gradient(closest-side, ${SHOWCASE.blue}, transparent)` }}
      />
      <div className="relative mx-auto max-w-3xl">
        <h2 id="job-summary-title" className="text-5xl font-semibold leading-none tracking-[-0.03em] md:text-7xl">
          {TEASER_TITLE}
        </h2>
        <p className="mx-auto mt-6 max-w-xl text-lg leading-snug md:text-xl" style={{ color: SHOWCASE.mutedOnDark }}>
          {TEASER_LEAD}
        </p>
        <p data-testid="job-summary-next" className="mt-4 text-base font-medium text-white">
          {TEASER_NEXT[next]}
        </p>
        <ol data-testid="job-summary-steps" aria-label="The first steps" className="mt-8 flex items-center justify-center gap-3">
          {FIRST_STEPS.map((spec, i) => {
            const done = steps.find((s) => s.id === spec.id)?.done === true
            return (
              <li key={spec.id} className="flex items-center gap-2 text-sm" data-state={done ? 'done' : 'open'} title={spec.title}>
                <span
                  aria-hidden="true"
                  className="flex h-7 w-7 items-center justify-center rounded-full text-xs font-semibold"
                  style={{ background: done ? SHOWCASE.blue : 'rgba(255,255,255,0.14)', color: '#ffffff' }}
                >
                  {done ? '✓' : i + 1}
                </span>
                <span className="hidden text-white/80 sm:inline">{spec.title}</span>
              </li>
            )
          })}
        </ol>
        <div className="mt-10 flex flex-wrap items-center justify-center gap-3">
          <ShowcasePill onClick={onOpen} arrow>
            {TEASER_BUTTONS.info}
          </ShowcasePill>
          <ShowcasePill tone="outline-dark" href={PLATFORMS_PATH}>
            {next === 'platform' ? TEASER_BUTTONS.platform : TEASER_BUTTONS.platforms}
          </ShowcasePill>
          <ShowcasePill tone="outline-dark" onClick={onAddJob}>
            {TEASER_BUTTONS.add}
          </ShowcasePill>
        </div>
        <p className="mt-8 text-sm">
          <AppLink href={GUIDE_LINK.path} className="text-[#2997ff] hover:underline">
            {GUIDE_LINK.label} →
          </AppLink>
        </p>
      </div>
    </section>
  )
}
