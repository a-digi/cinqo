import { PAGE_ACCENT, type Accent } from '../../Components/JobsPage/jobsTheme'
import { AppLink } from '../AppLink/AppLink'
import { isLast, nodeLook, numberLabel, type NodeLook, type TimelineStep } from './timelineRules'

// The look of the number badge for each state of a step. A step with no state is drawn as a filled, numbered badge.
function badgeStyle(look: NodeLook, accent: Accent): React.CSSProperties {
  switch (look) {
    case 'done':
      return { background: '#16a34a', color: '#ffffff' }
    case 'upcoming':
      return { background: '#ffffff', color: accent.text, border: `2px solid ${accent.from}` }
    case 'current':
      return {
        background: `linear-gradient(135deg, ${accent.from}, ${accent.to})`,
        color: '#ffffff',
        boxShadow: `0 0 0 4px ${accent.soft}`,
      }
    case 'neutral':
      return { background: `linear-gradient(135deg, ${accent.from}, ${accent.to})`, color: '#ffffff' }
  }
}

// The shared timeline (plan/ai/tools/career/step-170-onboarding-guide.md): a vertical line with a numbered badge for each step, so a
// list of steps reads as a sequence that happens in order. It only draws what it is given: a title, a text, an optional tip and an
// optional link into another page of the tool for each step. It knows nothing about what the steps are about. A screen reader gets an
// ordered list; a step marked 'current' is announced as the current step.
export function Timeline({
  steps,
  ariaLabel,
  accent = PAGE_ACCENT,
}: {
  steps: readonly TimelineStep[]
  ariaLabel: string
  accent?: Accent
}) {
  return (
    <ol aria-label={ariaLabel} className="career-stagger m-0 list-none p-0">
      {steps.map((step, index) => {
        const look = nodeLook(step)
        const last = isLast(index, steps.length)
        return (
          <li key={step.id} aria-current={look === 'current' ? 'step' : undefined} className={`flex gap-4 ${last ? '' : 'pb-6'}`}>
            <div className="flex shrink-0 flex-col items-center">
              <span
                className="inline-flex h-9 w-9 items-center justify-center rounded-full text-sm font-bold shadow-sm"
                style={badgeStyle(look, accent)}
                aria-hidden="true"
              >
                {look === 'done' ? '✓' : numberLabel(index)}
              </span>
              {!last && <span aria-hidden="true" className="mt-1 w-0.5 flex-1 rounded-full" style={{ background: `${accent.from}66` }} />}
            </div>
            <div className="min-w-0 flex-1 pt-1">
              <h2 className="text-sm font-semibold text-gray-900">
                <span className="sr-only">{`Step ${numberLabel(index)}: `}</span>
                {step.title}
              </h2>
              <p className="mt-1 text-sm leading-relaxed text-gray-600">{step.text}</p>
              {step.tip !== undefined && (
                <p className="mt-2 rounded-xl px-3 py-2 text-xs leading-relaxed" style={{ background: accent.soft, color: accent.text }}>
                  <span className="font-semibold">Tip: </span>
                  {step.tip}
                </p>
              )}
              {step.link !== undefined && (
                <AppLink
                  href={step.link.path}
                  className="mt-2 inline-block text-sm font-medium hover:underline"
                  style={{ color: accent.text }}
                >
                  {step.link.label} →
                </AppLink>
              )}
            </div>
          </li>
        )
      })}
    </ol>
  )
}
