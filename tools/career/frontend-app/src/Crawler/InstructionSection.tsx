import type { ReactNode } from 'react'

// One numbered step of an instruction panel (plan/ai/tools/career/step-246-instruction-modal-two-columns-and-design.md): a dark number, a title, a line that says
// what to do, and the step's content under a thin divider from the step before. The first step has no divider.
export function InstructionSection({
  number,
  title,
  description,
  children,
}: {
  number: number
  title: string
  description?: string
  children: ReactNode
}) {
  return (
    <section className={number === 1 ? '' : 'mt-8 border-t border-gray-200 pt-6'} data-testid={`instruction-step-${String(number)}`}>
      <div className="flex items-start gap-3">
        <span
          aria-hidden="true"
          className="mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-gray-900 text-xs font-semibold text-white"
        >
          {number}
        </span>
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-semibold text-gray-900">{title}</h3>
          {description !== undefined && <p className="mt-0.5 text-xs text-gray-500">{description}</p>}
          <div className="mt-4 space-y-3 text-sm text-gray-800 [&_input]:accent-gray-900">{children}</div>
        </div>
      </div>
    </section>
  )
}
