import { PAGE_ACCENT } from '../../JobsPage/jobsTheme'
import type { DetailsStep } from './detailsStats'

// The Summary & Goals page's overview: the six fields as a vertical stepper, in form
// order. A filled field has a green tick, an empty one an open circle; the line between
// two steps is green while the step above is filled. Clicking a step asks the page to
// focus that field. The foot has the persona's skill and experience counts.
export function FieldStepper({
  steps,
  skillCount,
  experienceCount,
  onPick,
  onOpenSkills,
  onOpenExperience,
}: {
  steps: DetailsStep[]
  skillCount: number
  experienceCount: number
  onPick: (key: DetailsStep['key']) => void
  onOpenSkills: () => void
  onOpenExperience: () => void
}) {
  const filled = steps.filter((s) => s.filled).length
  return (
    <section className="career-enter career-delay-2 rounded-3xl border bg-white p-4 shadow-sm" style={{ borderColor: PAGE_ACCENT.soft }}>
      <div className="flex items-baseline justify-between">
        <h2 className="text-sm font-semibold text-gray-800">Fields</h2>
        <span className="text-xs text-gray-500">
          {filled} of {steps.length}
        </span>
      </div>
      <ol className="mt-3">
        {steps.map((step, i) => (
          <li key={step.key} className="relative pb-3 pl-8 last:pb-0">
            {i < steps.length - 1 && (
              <span
                aria-hidden="true"
                className="career-draw absolute left-[11px] top-6 h-[calc(100%-1rem)] w-0.5"
                style={{ background: step.filled ? '#16a34a' : '#e5e7eb', '--i': i } as React.CSSProperties}
              />
            )}
            <span
              aria-hidden="true"
              className={`career-tick absolute left-0 top-0.5 flex h-6 w-6 items-center justify-center rounded-full border-2 text-xs font-bold ${
                step.filled ? 'border-green-600 bg-green-600 text-white' : 'border-gray-300 bg-white text-transparent'
              }`}
              style={{ '--i': i } as React.CSSProperties}
            >
              ✓
            </span>
            <button
              type="button"
              onClick={() => {
                onPick(step.key)
              }}
              className={`text-left text-sm hover:underline ${step.filled ? 'text-gray-700' : 'font-medium text-gray-900'}`}
            >
              {step.label}
              {!step.filled && <span className="ml-1.5 text-xs font-normal text-amber-700">empty</span>}
            </button>
          </li>
        ))}
      </ol>
      <div className="mt-4 flex flex-wrap gap-2 border-t pt-3" style={{ borderColor: PAGE_ACCENT.soft }}>
        <button
          type="button"
          onClick={onOpenSkills}
          className="rounded-full px-2.5 py-0.5 text-xs font-medium hover:underline"
          style={{ background: PAGE_ACCENT.soft, color: PAGE_ACCENT.text }}
        >
          {skillCount} {skillCount === 1 ? 'skill' : 'skills'} →
        </button>
        <button
          type="button"
          onClick={onOpenExperience}
          className="rounded-full bg-violet-50 px-2.5 py-0.5 text-xs font-medium text-violet-700 hover:underline"
        >
          {experienceCount} {experienceCount === 1 ? 'role' : 'roles'} →
        </button>
      </div>
    </section>
  )
}
