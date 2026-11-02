import type { Accent } from '../../Components/JobsPage/jobsTheme'
import { AppLink } from '../AppLink/AppLink'
import { badgeStyle } from './badgeStyle'
import { nodeLook, numberLabel, type TimelineStep } from './timelineRules'

// The round number badge (a ✓ for a done step).
export function StepBadge({
  step,
  index,
  accent,
  className = 'h-9 w-9 text-sm',
}: {
  step: TimelineStep
  index: number
  accent: Accent
  className?: string
}) {
  const look = nodeLook(step)
  return (
    <span
      className={`inline-flex shrink-0 items-center justify-center rounded-full font-bold shadow-sm ${className}`}
      style={badgeStyle(look, accent)}
      aria-hidden="true"
    >
      {look === 'done' ? '✓' : numberLabel(index)}
    </span>
  )
}

// The words of a step, the same in every design: the title (hidden when the design draws it itself), the text, the tip in the step's own
// tint and the link in its own colour. A screen reader hears "Step N:" before the title.
export function StepWords({
  step,
  index,
  accent,
  hideTitle = false,
}: {
  step: TimelineStep
  index: number
  accent: Accent
  hideTitle?: boolean
}) {
  return (
    <>
      {!hideTitle && (
        <h2 className="text-sm font-semibold text-gray-900">
          <span className="sr-only">{`Step ${numberLabel(index)}: `}</span>
          {step.title}
        </h2>
      )}
      <p className={`${hideTitle ? '' : 'mt-1 '}text-sm leading-relaxed text-gray-600`}>{step.text}</p>
      {step.tip !== undefined && (
        <p
          className="mt-2 rounded-xl px-3 py-2 text-xs leading-relaxed md:text-left"
          style={{ background: accent.soft, color: accent.text }}
        >
          <span className="font-semibold">Tip: </span>
          {step.tip}
        </p>
      )}
      {step.link !== undefined && (
        <AppLink href={step.link.path} className="mt-2 inline-block text-sm font-medium hover:underline" style={{ color: accent.text }}>
          {step.link.label} →
        </AppLink>
      )}
    </>
  )
}
