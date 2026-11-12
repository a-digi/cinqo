import { useCallback, useEffect, useRef, useState } from 'react'
import { AppLink } from '../AppLink/AppLink'
import { Decor } from '../Decor/Decor'
import { Modal } from '../Modal/Modal'
import { gradient, type Accent } from '../../Components/JobsPage/jobsTheme'
import { chapterInView } from './infographicRules'
import type { Chapter } from './infographicTypes'
import { Reveal } from './Reveal'

// The element that scrolls the poster: the nearest ancestor that scrolls (the body of the full-page modal).
function scrollParentOf(el: HTMLElement | null): HTMLElement | null {
  for (let p = el?.parentElement ?? null; p; p = p.parentElement) {
    const overflow = getComputedStyle(p).overflowY
    if (overflow === 'auto' || overflow === 'scroll') return p
  }
  return null
}

function prefersReducedMotion(): boolean {
  return (
    typeof window !== 'undefined' &&
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches
  )
}

// Where the chapter bar's own height ends: a chapter counts as "in view" once its top has passed this line.
const BAR_LINE = 96

export interface InfographicProps<Id extends string> {
  open: boolean
  onClose: () => void
  // The title of the full-page modal.
  title: string
  chapters: Chapter<Id>[]
  accentOf: (id: Id) => Accent
  // The start of every id on the poster ("platform-info": "platform-info-crawler" and "platform-info-crawler-title").
  idPrefix: string
  // The first chapter's own heading: a small line above its title and a longer lead instead of its promise.
  hero?: { chapterId: Id; kicker: string; lead: string }
  // A small badge on a chapter's button in the bar (such as "2/4"), or null.
  badge?: (id: Id) => string | null
  // The picture of a chapter; `go` scrolls to another chapter.
  renderPicture: (id: Id, go: (id: Id) => void) => React.ReactNode
}

// A "How it works" infographic in a full-page modal (plan 262, shared by plan 266): a sticky bar of chapters (the one in view is marked and a click scrolls
// to it), then the chapters one under the other, each with its picture, caption cards, "where to click" chips and links.
export function InfographicModal<Id extends string>(props: InfographicProps<Id>) {
  return (
    <Modal open={props.open} title={props.title} onClose={props.onClose} size="full">
      <Poster {...props} />
    </Modal>
  )
}

function Poster<Id extends string>({ chapters, accentOf, idPrefix, hero, badge, renderPicture }: InfographicProps<Id>) {
  const root = useRef<HTMLDivElement>(null)
  const sections = useRef<Partial<Record<Id, HTMLElement | null>>>({})
  const [current, setCurrent] = useState(0)

  // The chapter in view follows the scroll.
  useEffect(() => {
    const scroller = scrollParentOf(root.current)
    if (!scroller) return
    const update = () => {
      const top = scroller.getBoundingClientRect().top
      const tops = chapters.map((c) => (sections.current[c.id]?.getBoundingClientRect().top ?? Number.POSITIVE_INFINITY) - top)
      setCurrent(chapterInView(tops, BAR_LINE))
    }
    update()
    scroller.addEventListener('scroll', update, { passive: true })
    return () => {
      scroller.removeEventListener('scroll', update)
    }
  }, [chapters])

  const go = useCallback((id: Id) => {
    sections.current[id]?.scrollIntoView({ behavior: prefersReducedMotion() ? 'auto' : 'smooth', block: 'start' })
  }, [])

  return (
    <div ref={root} className="mx-auto max-w-5xl pb-16">
      <nav
        aria-label="Chapters"
        className="sticky -top-5 z-10 -mx-6 -mt-5 mb-6 border-b border-gray-200 bg-white/95 px-6 py-2.5 backdrop-blur"
      >
        <ul className="flex gap-1.5 overflow-x-auto">
          {chapters.map((c, i) => {
            const accent = accentOf(c.id)
            const active = i === current
            const extra = badge?.(c.id) ?? null
            return (
              <li key={c.id} className="shrink-0">
                <button
                  type="button"
                  onClick={() => {
                    go(c.id)
                  }}
                  aria-current={active ? 'true' : undefined}
                  className={`flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs font-semibold transition-colors ${active ? 'text-white' : 'text-gray-600 hover:bg-gray-50'}`}
                  style={active ? { background: gradient(accent), borderColor: accent.to } : { borderColor: '#e5e7eb' }}
                >
                  <span
                    className={`flex h-4 w-4 items-center justify-center rounded-full text-[10px] ${active ? 'bg-white/25' : 'bg-gray-100'}`}
                  >
                    {i + 1}
                  </span>
                  {c.nav}
                  {extra !== null && <span className="rounded-full bg-black/10 px-1.5 text-[10px]">{extra}</span>}
                </button>
              </li>
            )
          })}
        </ul>
      </nav>

      <div className="mx-auto max-w-5xl space-y-20 pb-16">
        {chapters.map((c, i) => (
          <section
            key={c.id}
            id={`${idPrefix}-${c.id}`}
            ref={(el) => {
              sections.current[c.id] = el
            }}
            aria-labelledby={`${idPrefix}-${c.id}-title`}
            className="scroll-mt-20"
          >
            <ChapterHead
              chapter={c}
              index={i}
              accent={accentOf(c.id)}
              idPrefix={idPrefix}
              hero={hero?.chapterId === c.id ? hero : undefined}
            />
            <div className="my-8">{renderPicture(c.id, go)}</div>
            {c.captions.length > 0 && (
              <ul className={`grid gap-3 ${c.captions.length === 3 ? 'md:grid-cols-3' : 'md:grid-cols-2'}`}>
                {c.captions.map((cap, k) => (
                  <Reveal as="li" key={cap.title} delay={k % 3}>
                    <div className="h-full rounded-2xl border bg-white p-4 shadow-sm" style={{ borderColor: accentOf(c.id).soft }}>
                      <h4 className="text-sm font-semibold text-gray-900">{cap.title}</h4>
                      <p className="mt-1 text-sm leading-relaxed text-gray-600">{cap.text}</p>
                    </div>
                  </Reveal>
                ))}
              </ul>
            )}
            {(c.click.length > 0 || c.links.length > 0) && (
              <div className="mt-4 flex flex-wrap items-center gap-2 text-xs text-gray-600">
                {c.click.length > 0 && (
                  <>
                    <span className="font-semibold uppercase tracking-wide text-gray-400">Where to click</span>
                    {c.click.map((b) => (
                      <span key={b} className="rounded-full border border-gray-300 bg-white px-3 py-1 font-medium text-gray-800 shadow-sm">
                        {b}
                      </span>
                    ))}
                  </>
                )}
                {c.links.map((l) => (
                  <AppLink
                    key={l.path}
                    href={l.path}
                    className="rounded-full px-3 py-1 font-semibold hover:underline"
                    style={{ background: accentOf(c.id).soft, color: accentOf(c.id).text }}
                  >
                    {l.label} →
                  </AppLink>
                ))}
              </div>
            )}
          </section>
        ))}
      </div>
    </div>
  )
}

function ChapterHead({
  chapter,
  index,
  accent,
  idPrefix,
  hero,
}: {
  chapter: Chapter
  index: number
  accent: Accent
  idPrefix: string
  hero: { kicker: string; lead: string } | undefined
}) {
  return (
    <header
      className="relative overflow-hidden rounded-3xl border p-6"
      style={{ borderColor: accent.soft, background: `linear-gradient(160deg, #ffffff 40%, ${accent.soft} 100%)` }}
    >
      <Decor shape={index % 3 === 0 ? 'ring' : index % 3 === 1 ? 'diamond' : 'circles'} accent={accent} />
      <div className="relative flex items-start gap-4">
        <span
          className="flex h-12 w-12 shrink-0 items-center justify-center rounded-2xl text-xl font-bold text-white shadow-sm"
          style={{ background: gradient(accent) }}
          aria-hidden="true"
        >
          {index + 1}
        </span>
        <div className="min-w-0">
          {hero && <p className="text-xs font-semibold uppercase tracking-wider text-gray-400">{hero.kicker}</p>}
          <h3 id={`${idPrefix}-${chapter.id}-title`} className="text-2xl font-bold text-gray-900">
            {chapter.title}
          </h3>
          <p className="mt-1 text-sm text-gray-600">{hero ? hero.lead : chapter.promise}</p>
          {hero && <p className="mt-1 text-sm text-gray-500">{chapter.promise}</p>}
        </div>
      </div>
    </header>
  )
}
