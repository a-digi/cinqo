import { SHOWCASE, toneColors, type ShowcaseTone } from './showcaseTokens'
import { pinnedSectionHeight, pinnedStep } from './pinnedRules'
import { usePinnedMode, useScrollProgress } from './useScrollProgress'

export interface PinnedStep {
  id: string
  title: string
  text: string
}

// A pinned stage (the showcase look's scroll-driven picture): a tall section with a stage that sticks to the top while the person scrolls through it; the steps are listed at the left, the
// one in turn is bright and the others dim, and the picture at the right is told which step it is and how far through (0 to 1). On a narrow window, and for a person who asks for less
// motion, there is no pinning: every step is shown as a plain list with the picture of the last step under it.
export function PinnedStage({
  idPrefix,
  steps,
  tone,
  picture,
}: {
  idPrefix: string
  steps: PinnedStep[]
  tone: ShowcaseTone
  picture: (step: number, progress: number) => React.ReactNode
}) {
  const pinned = usePinnedMode()
  const [ref, progress] = useScrollProgress<HTMLDivElement>(64)
  const c = toneColors(tone)
  const active = pinnedStep(progress, steps.length)

  if (!pinned) {
    return (
      <div data-testid="pinned-list" className="mx-auto max-w-3xl text-left">
        <ol className="space-y-8">
          {steps.map((s, i) => (
            <li key={s.id} id={`${idPrefix}-step-${s.id}`}>
              <p className="text-sm font-semibold" style={{ color: SHOWCASE.blue }}>
                {i + 1}
              </p>
              <h4 className="mt-1 text-3xl font-semibold tracking-tight" style={{ color: c.color }}>
                {s.title}
              </h4>
              <p className="mt-2 text-lg leading-snug" style={{ color: c.muted }}>
                {s.text}
              </p>
            </li>
          ))}
        </ol>
        <div className="mt-12">{picture(steps.length - 1, 1)}</div>
      </div>
    )
  }

  return (
    <div ref={ref} data-testid="pinned-section" style={{ height: pinnedSectionHeight(steps.length) }}>
      <div
        className="sticky top-16 grid h-[75vh] items-center gap-12 md:grid-cols-[minmax(0,1fr)_minmax(0,1.3fr)]"
        data-testid="pinned-stage"
      >
        <ol className="space-y-6 text-left">
          {steps.map((s, i) => {
            const on = i === active
            return (
              <li
                key={s.id}
                id={`${idPrefix}-step-${s.id}`}
                data-active={on ? 'true' : 'false'}
                aria-current={on ? 'step' : undefined}
                className="showcase-step"
                style={{ opacity: on ? 1 : 0.32 }}
              >
                <p className="text-sm font-semibold" style={{ color: SHOWCASE.blue }}>
                  {i + 1}
                </p>
                <h4 className="mt-1 text-3xl font-semibold tracking-tight md:text-4xl" style={{ color: c.color }}>
                  {s.title}
                </h4>
                <p className="mt-2 text-lg leading-snug" style={{ color: c.muted }}>
                  {s.text}
                </p>
              </li>
            )
          })}
        </ol>
        <div data-testid="pinned-picture" data-step={active}>
          {picture(active, progress)}
        </div>
      </div>
    </div>
  )
}
