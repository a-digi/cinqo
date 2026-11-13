import { SHOWCASE } from './showcaseTokens'

// The last band of the showcase look: one line set very large and the buttons.
export function ClosingLine({ line, actions }: { line: string; actions?: React.ReactNode }) {
  return (
    <div
      data-testid="closing-line"
      className="-mx-6 px-6 py-28 text-center md:py-40"
      style={{ background: SHOWCASE.black, color: SHOWCASE.white, fontFamily: SHOWCASE.font }}
    >
      <p className="mx-auto max-w-4xl text-5xl font-semibold leading-[1.05] tracking-[-0.03em] md:text-7xl">{line}</p>
      {actions !== undefined && <div className="mt-10 flex flex-wrap items-center justify-center gap-4">{actions}</div>}
    </div>
  )
}
