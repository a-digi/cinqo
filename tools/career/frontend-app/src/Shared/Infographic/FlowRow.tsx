import { gradient, type Accent } from '../../Components/JobsPage/jobsTheme'
import { Arrow } from './Arrow'
import { Reveal } from './Reveal'

// One node of a flow: what it is called, a few words, its icon and colour, and the chapter that zooms into it (none for a node without one).
export interface FlowNode {
  id: string
  label: string
  words: string
  chapter: string | null
  icon: React.ReactNode
  accent: Accent
}

// The big picture of an infographic: nodes joined by arrows (a row on a wide window, a column on a narrow one). A node that has a chapter of its own is a
// button that scrolls to it.
export function FlowRow({
  nodes,
  onGo,
  ariaLabel = 'The flow',
}: {
  nodes: FlowNode[]
  onGo: (chapter: string) => void
  ariaLabel?: string
}) {
  return (
    <ol className="flex flex-col items-stretch md:flex-row md:items-center" aria-label={ariaLabel}>
      {nodes.map((n, i) => {
        const accent = n.accent
        const chapter = n.chapter
        const body = (
          <>
            <span
              className="flex h-12 w-12 items-center justify-center rounded-2xl text-white shadow-sm"
              style={{ background: gradient(accent) }}
            >
              {n.icon}
            </span>
            <span className="mt-2 block text-sm font-semibold text-gray-900">{n.label}</span>
            <span className="block text-xs text-gray-500">{n.words}</span>
          </>
        )
        const box = 'flex h-full w-full flex-col items-center rounded-2xl border p-3 text-center'
        return (
          <li key={n.id} className="contents">
            <div className="min-w-0 flex-1 md:basis-0">
              <Reveal delay={i} className="h-full">
                {chapter !== null ? (
                  <button
                    type="button"
                    onClick={() => {
                      onGo(chapter)
                    }}
                    className={`${box} transition-transform hover:-translate-y-0.5 hover:shadow-md motion-reduce:transition-none motion-reduce:hover:translate-y-0`}
                    style={{ borderColor: accent.soft, background: accent.soft }}
                    aria-label={`${n.label}: ${n.words}. Go to the chapter`}
                  >
                    {body}
                  </button>
                ) : (
                  <div className={box} style={{ borderColor: accent.soft, background: accent.soft }}>
                    {body}
                  </div>
                )}
              </Reveal>
            </div>
            {i < nodes.length - 1 && (
              <div aria-hidden="true" className="flex justify-center py-1 md:px-1 md:py-0">
                {/* The hiding is done by these wrappers: the arrow's own display rule (career-info-arrow) would win over a hide class put on it. */}
                <span className="md:hidden">
                  <Arrow accent={accent} down />
                </span>
                <span className="hidden md:inline-flex">
                  <Arrow accent={accent} />
                </span>
              </div>
            )}
          </li>
        )
      })}
    </ol>
  )
}
