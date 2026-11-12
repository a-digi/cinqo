import {
  BriefcaseIcon,
  CheckCircleIcon,
  GlobeIcon,
  LinkGlyphIcon,
  LockIcon,
  TargetIcon,
  PulseIcon,
  RobotIcon,
} from '../../../../Shared/Icons/icons'
import { ACCENTS, gradient, type Accent } from '../../../JobsPage/jobsTheme'
import { CRAWL_STEPS, ENGINE_CARDS, HERO_NODES, LINK_PARTS, TROUBLE, FIRST_STEPS, type ChapterId } from '../platformInfoData'
import type { JourneyStep } from '../platformInfoRules'
import { Reveal } from '../Reveal'
import { useSeen } from '../useSeen'

// The pictures of the infographic (plan 262). Boxes and text are real HTML (so they are read aloud and scale with the text size); only the arrows and
// the little shapes are SVG, and those are hidden from assistive technology. Colours come from the tool's palette.

// An arrow made of HTML and CSS (a line and a head): it grows once when its chapter has come into view. `down` points down (used where a row wraps).
export function Arrow({ accent, down = false, className = '' }: { accent: Accent; down?: boolean; className?: string }) {
  // It starts growing when it itself comes into view (it may sit outside any revealed block).
  const [ref, seen] = useSeen<HTMLSpanElement>()
  return (
    <span
      ref={ref}
      aria-hidden="true"
      className={`career-info-arrow shrink-0 ${seen ? 'is-in' : ''} ${down ? 'is-down' : ''} ${className}`}
      style={{ color: accent.to }}
    >
      <i className="career-info-arrow-line" />
      <i className="career-info-arrow-head" />
    </span>
  )
}

const HERO_ICONS: Record<string, React.ReactNode> = {
  platform: <GlobeIcon className="h-6 w-6" />,
  links: <LinkGlyphIcon className="h-6 w-6" />,
  crawler: <RobotIcon className="h-6 w-6" />,
  jobs: <BriefcaseIcon className="h-6 w-6" />,
  matches: <TargetIcon className="h-6 w-6" />,
}

const HERO_ACCENTS: Accent[] = [ACCENTS[0], ACCENTS[6], ACCENTS[5], ACCENTS[1], ACCENTS[4]]

// The big picture: five nodes joined by arrows. A node that has a chapter of its own is a button that scrolls to it.
export function HeroFlow({ onGo }: { onGo: (chapter: ChapterId) => void }) {
  return (
    <ol className="flex flex-col items-stretch md:flex-row md:items-center" aria-label="The flow">
      {HERO_NODES.map((n, i) => {
        const accent = HERO_ACCENTS[i]
        const chapter = n.chapter
        const body = (
          <>
            <span
              className="flex h-12 w-12 items-center justify-center rounded-2xl text-white shadow-sm"
              style={{ background: gradient(accent) }}
            >
              {HERO_ICONS[n.id]}
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
            {i < HERO_NODES.length - 1 && (
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

// A platform: a browser window with a site name, and its links stacked under it.
export function PlatformStack() {
  const accent = ACCENTS[0]
  const links = ['Backend jobs in Hamburg', 'Remote Go roles', 'Team lead positions']
  return (
    <div className="mx-auto w-full max-w-md overflow-hidden rounded-2xl border bg-white shadow-sm" style={{ borderColor: accent.from }}>
      <div className="flex items-center gap-1.5 border-b px-3 py-2" style={{ background: accent.soft, borderColor: accent.soft }}>
        <span className="h-2.5 w-2.5 rounded-full bg-rose-300" aria-hidden="true" />
        <span className="h-2.5 w-2.5 rounded-full bg-amber-300" aria-hidden="true" />
        <span className="h-2.5 w-2.5 rounded-full bg-green-300" aria-hidden="true" />
        <span className="ml-2 flex items-center gap-1 text-xs font-semibold" style={{ color: accent.text }}>
          <GlobeIcon className="h-3.5 w-3.5" /> Acme careers
        </span>
      </div>
      <ul className="space-y-2 p-3">
        {links.map((l, i) => (
          <Reveal as="li" key={l} delay={i + 1}>
            <div className="flex items-center gap-2 rounded-xl border border-gray-100 px-3 py-2 text-sm text-gray-700">
              <LinkGlyphIcon className="h-4 w-4 shrink-0" />
              <span className="min-w-0 flex-1 truncate">{l}</span>
              <span className="rounded-full bg-gray-100 px-2 py-0.5 text-[10px] font-medium text-gray-500">link</span>
            </div>
          </Reveal>
        ))}
      </ul>
    </div>
  )
}

// A link taken apart: the card the page shows, with its parts named under it.
export function LinkAnatomy() {
  const accent = ACCENTS[6]
  return (
    <div className="mx-auto w-full max-w-2xl">
      <div className="rounded-2xl border bg-white p-4 shadow-sm" style={{ borderColor: accent.from }}>
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-semibold text-gray-900">Backend jobs in Hamburg</span>
          <span className="rounded-full bg-green-100 px-2 py-0.5 text-xs font-medium text-green-800">Instructed</span>
          <span className="inline-flex items-center gap-1 rounded-full bg-violet-100 px-2 py-0.5 text-xs font-medium text-violet-800">
            <RobotIcon className="h-3 w-3" /> AI
          </span>
          <span className="inline-flex items-center gap-1 rounded-full bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-800">
            <LockIcon className="h-3 w-3" /> Login needed
          </span>
        </div>
        <p className="mt-1 truncate font-mono text-xs text-gray-500">https://jobs.example.com/search?q=backend&amp;city=hamburg</p>
      </div>
      <ul className="mt-3 grid gap-2 sm:grid-cols-5">
        {LINK_PARTS.map((p, i) => (
          <Reveal as="li" key={p.id} delay={i}>
            <div className="h-full rounded-xl p-2.5 text-center" style={{ background: accent.soft }}>
              <div className="text-xs font-semibold" style={{ color: accent.text }}>
                {p.label}
              </div>
              <div className="mt-0.5 text-[11px] leading-snug text-gray-600">{p.words}</div>
            </div>
          </Reveal>
        ))}
      </ul>
    </div>
  )
}

// The crawl as a row of steps; a dot travels along the row while the chapter is on screen, and the first step pulses.
export function CrawlTimeline() {
  const accent = ACCENTS[5]
  return (
    <div className="mx-auto w-full max-w-4xl">
      <div className="relative mb-3 hidden h-3 md:block" aria-hidden="true">
        <div className="absolute inset-x-0 top-1 h-1 rounded-full" style={{ background: accent.soft }} />
        <span className="career-info-run absolute top-0 h-3 w-3 rounded-full" style={{ background: gradient(accent) }} />
      </div>
      <ol className="grid gap-3 md:grid-cols-5">
        {CRAWL_STEPS.map((s, i) => (
          <Reveal as="li" key={s.id} delay={i}>
            <div className="relative h-full rounded-2xl border bg-white p-3 text-center shadow-sm" style={{ borderColor: accent.from }}>
              <span
                className={`mx-auto flex h-8 w-8 items-center justify-center rounded-full text-sm font-bold text-white ${i === 0 ? 'career-info-pulse' : ''}`}
                style={{ background: gradient(accent) }}
              >
                {i + 1}
              </span>
              <div className="mt-2 text-sm font-semibold text-gray-900">{s.label}</div>
              <div className="text-xs text-gray-500">{s.words}</div>
              {i === 2 && (
                <span
                  className="absolute -top-2 right-2 rounded-full px-2 py-0.5 text-[10px] font-semibold text-white"
                  style={{ background: accent.to }}
                >
                  repeats
                </span>
              )}
            </div>
          </Reveal>
        ))}
      </ol>
      <p className="mt-3 flex items-center justify-center gap-2 text-xs text-gray-500">
        <PulseIcon className="h-4 w-4" /> All of this runs on the server: you can close the page.
      </p>
    </div>
  )
}

// The two halves of a link's instructions: the job list and the job details, each saying what it tells the crawler.
export function InstructionRecipe() {
  const accent = ACCENTS[4]
  const halves = [
    { id: 'list', title: 'Job list', lines: ['Where the postings are on the page', 'How to turn to the next page'] },
    {
      id: 'detail',
      title: 'Job details',
      lines: ['Where the title and the company are', 'Where the place, the date and the description are'],
    },
  ]
  return (
    <div className="mx-auto grid w-full max-w-3xl gap-3 sm:grid-cols-[1fr_auto_1fr] sm:items-stretch">
      {halves.map((h, i) => (
        <div key={h.id} className="contents">
          <Reveal delay={i * 2} className="h-full">
            <div className="h-full rounded-2xl border bg-white p-4 shadow-sm" style={{ borderColor: accent.from }}>
              <div
                className="mb-2 inline-block rounded-full px-2.5 py-0.5 text-xs font-semibold text-white"
                style={{ background: gradient(accent) }}
              >
                {h.title}
              </div>
              <ul className="space-y-1 text-sm text-gray-700">
                {h.lines.map((l) => (
                  <li key={l} className="flex items-start gap-1.5">
                    <CheckCircleIcon className="mt-0.5 h-4 w-4 shrink-0" />
                    {l}
                  </li>
                ))}
              </ul>
            </div>
          </Reveal>
          {i === 0 && (
            <div className="flex items-center justify-center text-2xl font-bold" style={{ color: accent.to }} aria-label="and">
              +
            </div>
          )}
        </div>
      ))}
    </div>
  )
}

// The two ways to read a page, side by side.
export function EnginesCompare({ note }: { note: string }) {
  const accents = [ACCENTS[4], ACCENTS[0]]
  return (
    <div className="mx-auto w-full max-w-3xl">
      <h4 className="mb-2 text-center text-sm font-semibold text-gray-900">Two ways to read a page</h4>
      <div className="grid gap-3 sm:grid-cols-2">
        {ENGINE_CARDS.map((e, i) => (
          <Reveal key={e.id} delay={i}>
            <div className="h-full rounded-2xl border bg-white p-4 shadow-sm" style={{ borderColor: accents[i].from }}>
              <div className="flex items-baseline gap-2">
                <span className="text-base font-bold" style={{ color: accents[i].text }}>
                  {e.name}
                </span>
                <span className="text-xs text-gray-500">{e.tagline}</span>
              </div>
              <ul className="mt-2 space-y-1.5 text-sm text-gray-700">
                {e.points.map((p) => (
                  <li key={p} className="flex items-start gap-1.5">
                    <span className="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full" style={{ background: accents[i].to }} aria-hidden="true" />
                    {p}
                  </li>
                ))}
              </ul>
            </div>
          </Reveal>
        ))}
      </div>
      <p className="mt-2 text-center text-xs text-gray-500">{note}</p>
    </div>
  )
}

// "Your first platform": the six steps as a numbered path with what the page can tell (done, next, or not known), and a count.
export function FirstStepsPath({
  steps,
  progress,
  nextId,
}: {
  steps: JourneyStep[]
  progress: { done: number; total: number }
  nextId: string | null
}) {
  const accent = ACCENTS[1]
  return (
    <div className="mx-auto w-full max-w-3xl">
      <p role="status" className="mb-3 text-center text-sm text-gray-600">
        {progress.done === progress.total ? (
          <span className="font-semibold text-green-700">Every step you can see on this page is done.</span>
        ) : (
          <>
            <span className="font-semibold text-gray-900">
              {progress.done} of {progress.total}
            </span>{' '}
            steps you can see on this page are done.
          </>
        )}
      </p>
      <ol className="space-y-2.5">
        {FIRST_STEPS.map((s, i) => {
          const state = steps.find((x) => x.id === s.id)
          const done = state?.done === true
          const next = nextId === s.id
          return (
            <Reveal as="li" key={s.id} delay={i}>
              <div
                className="flex items-start gap-3 rounded-2xl border bg-white p-3 shadow-sm"
                style={{ borderColor: done ? accent.from : next ? ACCENTS[2].from : '#e5e7eb', background: done ? accent.soft : undefined }}
                data-state={done ? 'done' : next ? 'next' : 'open'}
              >
                <span
                  className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-sm font-bold text-white"
                  style={{ background: done ? gradient(accent) : next ? gradient(ACCENTS[2]) : '#9ca3af' }}
                  aria-hidden="true"
                >
                  {done ? '✓' : i + 1}
                </span>
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-sm font-semibold text-gray-900">{s.title}</span>
                    {done && <span className="rounded-full bg-green-100 px-2 py-0.5 text-[11px] font-medium text-green-800">Done</span>}
                    {next && <span className="rounded-full bg-amber-100 px-2 py-0.5 text-[11px] font-medium text-amber-800">Next</span>}
                  </div>
                  <p className="text-xs text-gray-600">{s.text}</p>
                </div>
                <span className="shrink-0 rounded-full border border-gray-200 bg-white px-2.5 py-1 text-xs font-medium text-gray-700">
                  {s.click}
                </span>
              </div>
            </Reveal>
          )
        })}
      </ol>
    </div>
  )
}

const PILL_STYLE: Record<string, string> = {
  'not-instructed': 'bg-red-100 text-red-800',
  'details-missing': 'bg-amber-100 text-amber-800',
  'not-crawlable': 'bg-red-100 text-red-800',
  'probably-not': 'bg-red-100 text-red-800',
  'zero-jobs': 'bg-orange-100 text-orange-800',
  question: 'bg-blue-100 text-blue-800',
  login: 'bg-slate-100 text-slate-800',
}

// What each message on a link means and what to do, as rows.
export function TroubleTable() {
  return (
    <ul className="mx-auto w-full max-w-4xl space-y-2.5">
      {TROUBLE.map((t, i) => (
        <Reveal as="li" key={t.id} delay={i % 3}>
          <div className="grid gap-2 rounded-2xl border border-gray-200 bg-white p-3 shadow-sm md:grid-cols-[13rem_1fr_1fr] md:items-start">
            <span
              className={`inline-flex w-fit rounded-full px-2.5 py-1 text-xs font-semibold ${PILL_STYLE[t.id] ?? 'bg-gray-100 text-gray-800'}`}
            >
              {t.pill}
            </span>
            <p className="text-sm text-gray-700">
              <span className="mr-1 text-[11px] font-semibold uppercase tracking-wide text-gray-400">It means</span>
              {t.meaning}
            </p>
            <p className="text-sm text-gray-700">
              <span className="mr-1 text-[11px] font-semibold uppercase tracking-wide text-gray-400">What to do</span>
              {t.todo}
            </p>
          </div>
        </Reveal>
      ))}
    </ul>
  )
}
