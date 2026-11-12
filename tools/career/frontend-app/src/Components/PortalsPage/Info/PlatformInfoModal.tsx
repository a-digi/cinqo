import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { AppLink } from '../../../Shared/AppLink/AppLink'
import { Modal } from '../../../Shared/Modal/Modal'
import { Decor } from '../../../Shared/Decor/Decor'
import { gradient } from '../../JobsPage/jobsTheme'
import {
  CrawlTimeline,
  EnginesCompare,
  FirstStepsPath,
  HeroFlow,
  InstructionRecipe,
  LinkAnatomy,
  PlatformStack,
  TroubleTable,
} from './diagrams/diagrams'
import { accentOfChapter } from './infoTheme'
import { CHAPTERS, ENGINES_NOTE, HERO_LEAD, HERO_TITLE, MODAL_TITLE, type Chapter, type ChapterId } from './platformInfoData'
import { chapterInView, journeyProgress, journeyState, nextJourneyStep, type JourneyFacts } from './platformInfoRules'
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

// "How platforms work" (plan 262): the whole infographic in a full-page modal. A sticky bar of chapters (the one in view is marked and a click
// scrolls to it), then the chapters one under the other: the big picture, platform, links, crawler, instructions, your first platform (marked with
// what the page can tell you has been done), and when it does not work. `facts` is what the Platforms page already knows.
export function PlatformInfoModal({ open, onClose, facts }: { open: boolean; onClose: () => void; facts: JourneyFacts }) {
  return (
    <Modal open={open} title={MODAL_TITLE} onClose={onClose} size="full">
      <Poster facts={facts} />
    </Modal>
  )
}

function Poster({ facts }: { facts: JourneyFacts }) {
  const root = useRef<HTMLDivElement>(null)
  const sections = useRef<Partial<Record<ChapterId, HTMLElement | null>>>({})
  const [current, setCurrent] = useState(0)
  const steps = useMemo(() => journeyState(facts), [facts])
  const progress = journeyProgress(facts)
  const nextId = nextJourneyStep(facts)

  // The chapter in view follows the scroll.
  useEffect(() => {
    const scroller = scrollParentOf(root.current)
    if (!scroller) return
    const update = () => {
      const top = scroller.getBoundingClientRect().top
      const tops = CHAPTERS.map((c) => (sections.current[c.id]?.getBoundingClientRect().top ?? Number.POSITIVE_INFINITY) - top)
      setCurrent(chapterInView(tops, BAR_LINE))
    }
    update()
    scroller.addEventListener('scroll', update, { passive: true })
    return () => {
      scroller.removeEventListener('scroll', update)
    }
  }, [])

  const go = useCallback((id: ChapterId) => {
    sections.current[id]?.scrollIntoView({ behavior: prefersReducedMotion() ? 'auto' : 'smooth', block: 'start' })
  }, [])

  return (
    <div ref={root} className="mx-auto max-w-5xl pb-16">
      <nav
        aria-label="Chapters"
        className="sticky -top-5 z-10 -mx-6 -mt-5 mb-6 border-b border-gray-200 bg-white/95 px-6 py-2.5 backdrop-blur"
      >
        <ul className="flex gap-1.5 overflow-x-auto">
          {CHAPTERS.map((c, i) => {
            const accent = accentOfChapter(c.id)
            const active = i === current
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
                  {c.id === 'first-steps' && (
                    <span className="rounded-full bg-black/10 px-1.5 text-[10px]">
                      {progress.done}/{progress.total}
                    </span>
                  )}
                </button>
              </li>
            )
          })}
        </ul>
      </nav>

      <div className="mx-auto max-w-5xl space-y-20 pb-16">
        {CHAPTERS.map((c, i) => (
          <section
            key={c.id}
            id={`platform-info-${c.id}`}
            ref={(el) => {
              sections.current[c.id] = el
            }}
            aria-labelledby={`platform-info-${c.id}-title`}
            className="scroll-mt-20"
          >
            <ChapterHead chapter={c} index={i} />
            <div className="my-8">
              {c.id === 'overview' && <HeroFlow onGo={go} />}
              {c.id === 'platform' && <PlatformStack />}
              {c.id === 'links' && <LinkAnatomy />}
              {c.id === 'crawler' && <CrawlTimeline />}
              {c.id === 'instructions' && (
                <div className="space-y-8">
                  <InstructionRecipe />
                  <EnginesCompare note={ENGINES_NOTE} />
                </div>
              )}
              {c.id === 'first-steps' && <FirstStepsPath steps={steps} progress={progress} nextId={nextId} />}
              {c.id === 'trouble' && <TroubleTable />}
            </div>
            {c.captions.length > 0 && (
              <ul className={`grid gap-3 ${c.captions.length === 3 ? 'md:grid-cols-3' : 'md:grid-cols-2'}`}>
                {c.captions.map((cap, k) => (
                  <Reveal as="li" key={cap.title} delay={k % 3}>
                    <div className="h-full rounded-2xl border bg-white p-4 shadow-sm" style={{ borderColor: accentOfChapter(c.id).soft }}>
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
                    style={{ background: accentOfChapter(c.id).soft, color: accentOfChapter(c.id).text }}
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

function ChapterHead({ chapter, index }: { chapter: Chapter; index: number }) {
  const accent = accentOfChapter(chapter.id)
  const hero = chapter.id === 'overview'
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
          {hero && <p className="text-xs font-semibold uppercase tracking-wider text-gray-400">{HERO_TITLE}</p>}
          <h3 id={`platform-info-${chapter.id}-title`} className="text-2xl font-bold text-gray-900">
            {chapter.title}
          </h3>
          <p className="mt-1 text-sm text-gray-600">{hero ? HERO_LEAD : chapter.promise}</p>
          {hero && <p className="mt-1 text-sm text-gray-500">{chapter.promise}</p>}
        </div>
      </div>
    </header>
  )
}
