export interface StepIndicatorProps {
  steps: readonly { label: string }[]
  currentStep: number
  // The furthest index the user has actually reached — every step up
  // to and including this one is clickable (revisit a finished step
  // freely); anything past it is locked (grayed out, not clickable),
  // per your own "can't skip ahead to a step he hasn't finished yet"
  // instruction. CvBuilderPage's own edit flow sets this to the LAST
  // index immediately (an existing document already has data for
  // every step, so nothing about it is actually "unfinished").
  furthestStep: number
  onStepClick: (index: number) => void
}

// The top-of-wizard progress stepper — CvBuilderPage's own "Step X of
// N: Label" text replaced with a real, clickable-where-allowed nav.
// See plan/ai/career/cv-builder/step-04-frontend-cv-builder.md.
export function StepIndicator({ steps, currentStep, furthestStep, onStepClick }: StepIndicatorProps) {
  return (
    <ol className="mb-6 flex items-center">
      {steps.map((s, i) => {
        const isCurrent = i === currentStep
        const isLocked = i > furthestStep
        const isDone = !isCurrent && !isLocked

        return (
          <li key={s.label} className="flex flex-1 items-center last:flex-none">
            <button
              type="button"
              onClick={() => {
                onStepClick(i)
              }}
              disabled={isLocked}
              className={`flex items-center gap-2 ${isLocked ? 'cursor-not-allowed' : 'cursor-pointer'}`}
            >
              <span
                className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-xs font-medium ${
                  isCurrent ? 'bg-gray-900 text-white' : isLocked ? 'bg-gray-100 text-gray-300' : 'border border-gray-900 text-gray-900'
                }`}
              >
                {isDone ? '✓' : i + 1}
              </span>
              <span
                className={`hidden text-xs font-medium sm:inline ${
                  isCurrent ? 'text-gray-900' : isLocked ? 'text-gray-300' : 'text-gray-600'
                }`}
              >
                {s.label}
              </span>
            </button>
            {i < steps.length - 1 && <span className={`mx-2 h-px flex-1 ${i < furthestStep ? 'bg-gray-900' : 'bg-gray-200'}`} />}
          </li>
        )
      })}
    </ol>
  )
}
