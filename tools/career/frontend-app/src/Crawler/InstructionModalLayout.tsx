import type { ReactNode } from 'react'
import { useIsWide } from './useIsWide'

// The body of an instruction modal (plan/ai/tools/career/step-246-instruction-modal-two-columns-and-design.md): the instructions on the left (75%) and
// the help on the right (25%), from 1024 px wide. The help column stays in view while the instructions scroll, and scrolls by itself when it is longer
// than the window. Narrower than that it is one column and the help sits under the instructions in a closed "Help" section (a quarter of a narrow
// window is too narrow to read).
export function InstructionModalLayout({
  children,
  help,
  helpTitle = 'Help',
}: {
  children: ReactNode
  help?: ReactNode
  helpTitle?: string
}) {
  const wide = useIsWide()
  if (help === undefined || help === null) return <div data-testid="instruction-layout">{children}</div>
  if (!wide) {
    return (
      <div data-testid="instruction-layout" data-layout="stacked">
        <div className="min-w-0">{children}</div>
        <details className="mt-8 rounded-xl border border-gray-200 bg-gray-50 p-4" data-testid="instruction-help-collapsed">
          <summary className="cursor-pointer text-sm font-semibold text-gray-900">{helpTitle}</summary>
          <div className="mt-3">{help}</div>
        </details>
      </div>
    )
  }
  return (
    <div className="grid grid-cols-4 items-start gap-8" data-testid="instruction-layout" data-layout="columns">
      <div className="col-span-3 min-w-0" data-testid="instruction-main">
        {children}
      </div>
      <aside
        className="sticky top-0 col-span-1 max-h-[calc(100vh-10rem)] overflow-y-auto rounded-xl bg-gray-50 p-4"
        aria-label={helpTitle}
        data-testid="instruction-help"
      >
        {help}
      </aside>
    </div>
  )
}
