import { ACCENTS, gradient, type Accent } from '../../Components/JobsPage/jobsTheme'
import { Decor } from '../Decor/Decor'
import { shapeAt } from '../Decor/decorShapes'
import { StepBadge, StepWords } from './StepParts'
import { PinsTimeline } from './PinsTimeline'
import { ZigzagTimeline } from './ZigzagTimeline'
import { nodeLook, numberLabel, onLeft, paletteIndex, type TimelineDesign, type TimelineStep } from './timelineRules'

// The shared timeline (plan/ai/tools/career/step-170-onboarding-guide.md): the steps of a guide drawn as a sequence that happens in
// order. Every step has its own colour (the palette follows the steps and starts again after the last) and the guide picks one of four
// designs (see TimelineDesign); the zigzag has its own component, the other three share this one. From a tablet up the steps sit in two equal columns and take turns left and right, each in its own
// grid row, the right ones pulled up into the empty half of the row above so the cards interlock; a dashed line runs between the columns
// and the dashboard's soft shapes sit behind each card. On a phone every card is one below the other. It only draws what it is given and
// knows nothing about what the steps are about. A screen reader gets an ordered list in step order; a step marked 'current' is announced
// as the current step.
export function Timeline({
  steps,
  ariaLabel,
  design = 'cards',
  palette = ACCENTS,
}: {
  steps: readonly TimelineStep[]
  ariaLabel: string
  design?: TimelineDesign
  palette?: readonly Accent[]
}) {
  const accentOf = (index: number): Accent => palette[paletteIndex(index, palette.length)]
  if (design === 'zigzag') return <ZigzagTimeline steps={steps} ariaLabel={ariaLabel} accentOf={accentOf} />
  if (design === 'pins') return <PinsTimeline steps={steps} ariaLabel={ariaLabel} accentOf={accentOf} />
  return (
    <div className="relative">
      {/* The dashed line between the two columns, behind the dots of the cards. */}
      <span
        aria-hidden="true"
        className="pointer-events-none absolute inset-y-4 left-1/2 hidden border-l-2 border-dashed border-gray-200 md:block"
      />
      <ol aria-label={ariaLabel} className="career-stagger m-0 grid list-none grid-cols-1 gap-x-6 gap-y-4 p-0 md:grid-cols-2">
        {steps.map((step, index) => {
          const accent = accentOf(index)
          const look = nodeLook(step)
          const left = onLeft(index)
          const number = look === 'done' ? '✓' : numberLabel(index)
          const shapes = <Decor shape={shapeAt(index)} accent={accent} />
          const title = (extra: string) => (
            <h2 className={`font-semibold ${extra}`}>
              <span className="sr-only">{`Step ${numberLabel(index)}: `}</span>
              {step.title}
            </h2>
          )
          // The small dot where the card meets the dashed line.
          const dot = (
            <span
              aria-hidden="true"
              className={`absolute top-4 hidden h-3 w-3 rounded-full ring-4 ring-white md:block ${left ? 'md:-right-[1.375rem]' : 'md:-left-[1.375rem]'}`}
              style={{ background: gradient(accent) }}
            />
          )

          let card: React.ReactNode
          if (design === 'cards') {
            card = (
              <div
                className="relative overflow-hidden rounded-3xl border p-5 pl-6 shadow-sm transition-shadow hover:shadow-md"
                style={{ borderColor: accent.soft, background: `linear-gradient(160deg, #ffffff 45%, ${accent.soft} 100%)` }}
              >
                <span aria-hidden="true" className="absolute inset-y-0 left-0 w-2" style={{ background: gradient(accent) }} />
                {shapes}
                <div className="relative flex items-start gap-3">
                  <StepBadge step={step} index={index} accent={accent} />
                  <div className="min-w-0 flex-1 pt-1">
                    <StepWords step={step} index={index} accent={accent} />
                  </div>
                </div>
              </div>
            )
          } else {
            card = (
              <div
                className="relative overflow-hidden rounded-3xl border-2 border-dashed p-5 transition-shadow hover:shadow-md"
                style={{ borderColor: accent.from, background: accent.soft }}
              >
                {shapes}
                <div className="relative">
                  <div className="mb-2 flex items-center gap-3">
                    <span
                      className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-xl text-sm font-bold text-white"
                      style={{ background: gradient(accent) }}
                      aria-hidden="true"
                    >
                      {number}
                    </span>
                    {title('text-base')}
                  </div>
                  <StepWords step={step} index={index} accent={accent} hideTitle />
                </div>
              </div>
            )
          }

          return (
            <li
              key={step.id}
              aria-current={look === 'current' ? 'step' : undefined}
              className={`relative ${left ? 'md:col-start-1' : 'md:col-start-2 md:-mt-12'}`}
              style={{ gridRow: index + 1 }}
            >
              {card}
              {dot}
            </li>
          )
        })}
      </ol>
    </div>
  )
}
