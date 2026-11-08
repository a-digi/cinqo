import type { Accent } from '../../Components/JobsPage/jobsTheme'
import { StepBadge, StepWords } from './StepParts'
import { isLast, nodeLook, onLeft, type TimelineStep } from './timelineRules'

// The zigzag design: one narrow list, the steps alternating left and right of a centre line with the numbered badge on it (one column on a
// phone). The connector runs from each step's colour to the next one's. Drawn exactly as it was before the two-sided designs came.
export function ZigzagTimeline({
  steps,
  ariaLabel,
  accentOf,
}: {
  steps: readonly TimelineStep[]
  ariaLabel: string
  accentOf: (index: number) => Accent
}) {
  return (
    <ol aria-label={ariaLabel} className="career-stagger m-0 list-none p-0">
      {steps.map((step, index) => {
        const accent = accentOf(index)
        const last = isLast(index, steps.length)
        const left = onLeft(index)
        const connector = last ? undefined : `linear-gradient(to bottom, ${accent.from}, ${accentOf(index + 1).from})`
        return (
          <li
            key={step.id}
            aria-current={nodeLook(step) === 'current' ? 'step' : undefined}
            className={`grid grid-cols-[auto_1fr] gap-x-4 md:grid-cols-[1fr_auto_1fr] ${last ? '' : 'pb-6'}`}
          >
            <div className="col-start-1 row-start-1 flex flex-col items-center md:col-start-2">
              <StepBadge step={step} index={index} accent={accent} />
              {connector !== undefined && (
                <span aria-hidden="true" className="mt-1 w-0.5 flex-1 rounded-full" style={{ background: connector }} />
              )}
            </div>
            <div className={`row-start-1 min-w-0 pt-1 ${left ? 'col-start-2 md:col-start-1 md:text-right' : 'col-start-2 md:col-start-3'}`}>
              <StepWords step={step} index={index} accent={accent} />
            </div>
          </li>
        )
      })}
    </ol>
  )
}
