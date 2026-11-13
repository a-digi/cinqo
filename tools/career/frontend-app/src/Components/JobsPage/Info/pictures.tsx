import {
  AlertCircleIcon,
  BriefcaseIcon,
  CheckCircleIcon,
  ClockIcon,
  FilterIcon,
  GlobeIcon,
  LayersIcon,
  LinkGlyphIcon,
} from '../../../Shared/Icons/icons'
import { FirstStepsPath } from '../../../Shared/Infographic/FirstStepsPath'
import type { JourneyStep } from '../../../Shared/Infographic/infographicTypes'
import { Reveal } from '../../../Shared/Infographic/Reveal'
import { SHOWCASE } from '../../../Shared/Infographic/showcase/showcaseTokens'
import {
  BOARD_COLUMNS,
  DETAIL_STATES,
  FIRST_STEPS,
  HERO_CARDS,
  MATCH_NUMBER,
  RULE_EXAMPLE,
  SAMPLE_PROFILES,
  STAT_CARDS,
  TEASER_BUTTONS,
  WAY_IN_STEPS,
} from './jobInfoData'

// The pictures of the jobs infographic (plan 269). Boxes, rings and text are real HTML and CSS (read aloud, scaling with the text size); the only SVGs are the tool's own small icons. The
// sample names and numbers are made up. A picture that depends on the band it sits on takes `tone`.

const dark = (tone: 'dark' | 'light') => tone === 'dark'

// A ring that is filled to `value` percent, with the number in the middle: a conic gradient (the `showcase-ring` class in index.css) fed by three custom properties, no drawing.
export function ScoreRing({ value, size = 56, tone = 'dark' }: { value: number; size?: number; tone?: 'dark' | 'light' }) {
  const v = Math.max(0, Math.min(100, Math.round(value)))
  return (
    <div
      data-testid="score-ring"
      data-value={v}
      role="img"
      aria-label={`${String(v)} percent match`}
      className="showcase-ring relative shrink-0 rounded-full"
      style={
        {
          width: size,
          height: size,
          '--ring': `${String(v * 3.6)}deg`,
          '--fill': SHOWCASE.blue,
          '--track': dark(tone) ? 'rgba(255,255,255,0.16)' : 'rgba(0,0,0,0.1)',
        } as React.CSSProperties
      }
    >
      <div
        className="absolute flex items-center justify-center rounded-full text-xs font-semibold"
        style={{
          inset: Math.max(4, Math.round(size * 0.12)),
          background: dark(tone) ? '#1d1d1f' : '#ffffff',
          color: dark(tone) ? '#ffffff' : '#1d1d1f',
          fontSize: Math.round(size * 0.26),
        }}
      >
        {v}
      </div>
    </div>
  )
}

function PlatformPill({ name, tone = 'dark' }: { name: string; tone?: 'dark' | 'light' }) {
  return (
    <span
      className="rounded-full px-2 py-0.5 text-[11px] font-medium"
      style={{ background: dark(tone) ? 'rgba(255,255,255,0.12)' : 'rgba(0,0,0,0.07)', color: dark(tone) ? '#d2d2d7' : '#424245' }}
    >
      {name}
    </span>
  )
}

// The hero's picture: three job cards fanned out and floating, each with a title, a platform and a match ring.
export function HeroCards() {
  return (
    <div data-testid="hero-cards" className="relative mx-auto h-72 w-full max-w-3xl md:h-80">
      {HERO_CARDS.map((c, i) => {
        const angle = (i - 1) * 7
        const shift = (i - 1) * 34
        return (
          <div
            key={c.title}
            data-testid="hero-card"
            className="showcase-float absolute left-1/2 top-4 w-60 rounded-3xl p-5 text-left shadow-2xl md:w-72"
            style={
              {
                marginLeft: shift * 3 - 120,
                transform: `rotate(${String(angle)}deg)`,
                zIndex: i === 1 ? 2 : 1,
                background: 'linear-gradient(160deg, #2a2a2d, #1d1d1f)',
                border: '1px solid rgba(255,255,255,0.12)',
                '--i': i,
              } as React.CSSProperties
            }
          >
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <p className="truncate text-lg font-semibold text-white">{c.title}</p>
                <div className="mt-2">
                  <PlatformPill name={c.platform} />
                </div>
              </div>
              <ScoreRing value={c.score} />
            </div>
            <div className="mt-5 space-y-2" aria-hidden="true">
              <div className="h-2 w-5/6 rounded-full bg-white/15" />
              <div className="h-2 w-2/3 rounded-full bg-white/10" />
            </div>
          </div>
        )
      })}
    </div>
  )
}

// One step of the way in, as a card: its title, a small drawing of what it is, and its state (done, active, waiting).
function WayCard({ index, step, progress }: { index: number; step: number; progress: number }) {
  const s = WAY_IN_STEPS[index]
  const state = index < step ? 'done' : index === step ? 'active' : 'waiting'
  // how far through its own step the stage is (0 to 1)
  const inStep = Math.max(0, Math.min(1, progress * WAY_IN_STEPS.length - index))
  return (
    <div
      data-testid={`way-card-${s.id}`}
      data-state={state}
      className="showcase-step rounded-3xl p-5 text-left"
      style={{
        background: '#ffffff',
        border: `1.5px solid ${state === 'active' ? SHOWCASE.blue : 'rgba(0,0,0,0.08)'}`,
        opacity: state === 'waiting' ? 0.35 : 1,
        transform: state === 'active' ? 'scale(1.02)' : 'none',
        transition: 'opacity 400ms, transform 400ms',
      }}
    >
      <div className="flex items-center gap-3 text-[#1d1d1f]">
        <span aria-hidden="true">
          {index === 0 ? (
            <GlobeIcon className="h-6 w-6" />
          ) : index === 1 ? (
            <LinkGlyphIcon className="h-6 w-6" />
          ) : index === 2 ? (
            <LayersIcon className="h-6 w-6" />
          ) : (
            <BriefcaseIcon className="h-6 w-6" />
          )}
        </span>
        <span className="text-base font-semibold">{s.title}</span>
        {state === 'done' && (
          <span data-testid="way-done" aria-hidden="true" className="ml-auto text-sm" style={{ color: SHOWCASE.blue }}>
            ✓
          </span>
        )}
      </div>
      <div className="mt-3 text-sm text-[#6e6e73]" aria-hidden="true">
        {index === 0 && <span className="rounded-full bg-black/5 px-3 py-1">Acme Careers · a job board</span>}
        {index === 1 && (
          <ul className="space-y-1">
            {['/jobs?team=engineering', '/jobs?region=hamburg', '/jobs?type=remote'].map((l) => (
              <li key={l} className="truncate rounded-lg bg-black/5 px-3 py-1">
                {l}
              </li>
            ))}
          </ul>
        )}
        {index === 2 && (
          <div>
            <div className="h-2 overflow-hidden rounded-full bg-black/10">
              <div
                data-testid="way-scan"
                className="h-full rounded-full"
                style={{ width: `${String(Math.round(inStep * 100))}%`, background: SHOWCASE.blue }}
              />
            </div>
            <p className="mt-2">Opens each page, finds the postings</p>
          </div>
        )}
        {index === 3 && (
          <ul className="space-y-1">
            {HERO_CARDS.map((c) => (
              <li key={c.title} className="flex items-center justify-between gap-2 rounded-lg bg-black/5 px-3 py-1">
                <span className="truncate">{c.title}</span>
                <PlatformPill name={c.platform} tone="light" />
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}

// The pinned stage's picture: the four steps as cards in a column; the ones behind are done, the one in turn is lit, the ones ahead wait.
export function WayPicture({ step, progress }: { step: number; progress: number }) {
  return (
    <div data-testid="way-picture" data-step={step} className="mx-auto w-full max-w-md space-y-3">
      {WAY_IN_STEPS.map((s, i) => (
        <WayCard key={s.id} index={i} step={step} progress={progress} />
      ))}
    </div>
  )
}

// The panel of the "two ways in" tabs: what the way looks like, drawn.
export function WayPanel({ id }: { id: 'crawled' | 'by-hand' }) {
  if (id === 'crawled') {
    return (
      <div data-testid="panel-crawled" className="mx-auto flex max-w-3xl flex-col items-center gap-4 md:flex-row">
        <div className="w-full rounded-3xl bg-white/10 p-5 text-left md:w-1/3">
          <p className="text-sm text-white/60">A link of a platform</p>
          <p className="mt-1 truncate font-medium text-white">/jobs?team=engineering</p>
        </div>
        <span aria-hidden="true" className="text-2xl text-white/60 md:rotate-0">
          →
        </span>
        <div className="w-full rounded-3xl bg-white p-5 text-left md:w-2/3">
          <div className="flex items-center justify-between gap-3">
            <p className="font-semibold text-[#1d1d1f]">Backend Engineer</p>
            <PlatformPill name="Acme Careers" tone="light" />
          </div>
          <p className="mt-1 text-sm text-[#6e6e73]">Saved by the crawler</p>
        </div>
      </div>
    )
  }
  return (
    <div data-testid="panel-by-hand" className="mx-auto max-w-md rounded-3xl bg-white p-6 text-left text-[#1d1d1f]">
      <p className="text-lg font-semibold">{TEASER_BUTTONS.add}</p>
      <div className="mt-4 space-y-3 text-sm">
        <div>
          <p className="text-xs text-[#6e6e73]">Title</p>
          <p className="rounded-xl border border-black/15 px-3 py-2">Product Designer</p>
        </div>
        <div>
          <p className="text-xs text-[#6e6e73]">Company</p>
          <p className="rounded-xl border border-black/15 px-3 py-2">Globex</p>
        </div>
        <p className="text-xs text-[#6e6e73]">Only the title is required.</p>
      </div>
    </div>
  )
}

// The match: a large ring and the same job scored against each profile, with the auto-add rule as a chip.
export function MatchPicture() {
  return (
    <div data-testid="match-picture" className="mx-auto flex max-w-3xl flex-col items-center gap-10 md:flex-row">
      <ScoreRing value={Number(MATCH_NUMBER.value)} size={180} />
      <div className="w-full flex-1 text-left">
        <ul className="space-y-4">
          {SAMPLE_PROFILES.map((p) => (
            <li key={p.name}>
              <div className="mb-1 flex justify-between text-base text-white">
                <span>{p.name}</span>
                <span>{p.score}%</span>
              </div>
              <div className="h-2 overflow-hidden rounded-full bg-white/15">
                <div
                  data-testid="match-bar"
                  className="h-full rounded-full"
                  style={{ width: `${String(p.score)}%`, background: p.score >= RULE_EXAMPLE.atLeast ? SHOWCASE.blue : '#6e6e73' }}
                />
              </div>
            </li>
          ))}
        </ul>
        <p data-testid="rule-chip" className="mt-6 inline-block rounded-full bg-white/10 px-4 py-2 text-sm text-white">
          At least {RULE_EXAMPLE.atLeast}% → {RULE_EXAMPLE.goesTo}
        </p>
      </div>
    </div>
  )
}

// The details: three jobs, each with the mark of its state, and the button that reads the missing ones.
export function DetailsPicture() {
  const marks: Record<(typeof DETAIL_STATES)[number]['id'], React.ReactNode> = {
    'not-crawled': <ClockIcon className="h-5 w-5" />,
    failed: <AlertCircleIcon />,
    crawled: <CheckCircleIcon className="h-5 w-5" />,
  }
  const colours = { 'not-crawled': '#6e6e73', failed: '#e11d48', crawled: '#16a34a' }
  return (
    <div data-testid="details-picture" className="mx-auto max-w-2xl space-y-3 text-left">
      {DETAIL_STATES.map((s, i) => (
        <div key={s.id} className="flex items-center gap-4 rounded-2xl bg-white p-4" data-state={s.id}>
          <span style={{ color: colours[s.id] }} aria-hidden="true">
            {s.id === 'failed' ? <span className="inline-flex [&>svg]:h-5 [&>svg]:w-5">{marks[s.id]}</span> : marks[s.id]}
          </span>
          <div className="min-w-0 flex-1">
            <p className="truncate font-medium text-[#1d1d1f]">{HERO_CARDS[i].title}</p>
            <p className="text-sm text-[#6e6e73]">{s.label}</p>
          </div>
        </div>
      ))}
      <p className="pt-2 text-center">
        <span className="inline-block rounded-full px-5 py-2.5 text-sm font-medium text-white" style={{ background: SHOWCASE.blue }}>
          Crawl missing details (2)
        </span>
      </p>
    </div>
  )
}

// Find the right one: the search box, the sort and the filters as a strip, and three statistic cards under it.
export function FindPicture() {
  return (
    <div data-testid="find-picture" className="mx-auto max-w-3xl text-left">
      <div className="flex flex-wrap items-center gap-2 rounded-2xl bg-white/10 p-3">
        <span className="min-w-[10rem] flex-1 rounded-xl bg-black/40 px-4 py-2 text-sm text-white/60">
          Search title, company, description…
        </span>
        <span className="rounded-full bg-white/15 px-4 py-2 text-sm text-white">Best match first</span>
        <span className="inline-flex items-center gap-1.5 rounded-full bg-white/15 px-4 py-2 text-sm text-white">
          <span aria-hidden="true" className="inline-flex [&>svg]:h-3.5 [&>svg]:w-3.5">
            <FilterIcon />
          </span>
          Filters
        </span>
      </div>
      <div className="mt-2 flex flex-wrap gap-2 text-xs text-white/70">
        {['Any location', 'Any company', 'Any platform'].map((f) => (
          <span key={f} className="rounded-full border border-white/25 px-3 py-1">
            {f}
          </span>
        ))}
      </div>
      <ul className="mt-6 grid gap-3 sm:grid-cols-3">
        {STAT_CARDS.map((c, i) => (
          <li key={c.id} data-testid="stat-mini" className="rounded-2xl bg-white/10 p-4">
            <p className="text-3xl font-semibold text-white">{['248', '186', '12'][i]}</p>
            <p className="mt-1 text-sm text-white/70">{c.label}</p>
          </li>
        ))}
      </ul>
    </div>
  )
}

// A profile's job board: its four fixed columns with a few small cards.
export function BoardPicture() {
  const cards: Record<string, string[]> = {
    'Not Reviewed': ['Backend Engineer', 'Team Lead'],
    Applied: ['Data Engineer'],
    Declined: [],
    'Stopped application': ['Product Designer'],
  }
  return (
    <div data-testid="board-picture" className="mx-auto grid max-w-4xl gap-3 text-left sm:grid-cols-2 lg:grid-cols-4">
      {BOARD_COLUMNS.map((c) => (
        <div key={c} data-testid="board-column" className="rounded-2xl bg-white p-3">
          <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-[#6e6e73]">{c}</p>
          <div className="space-y-2">
            {(cards[c] ?? []).map((t) => (
              <div key={t} className="rounded-xl bg-[#f5f5f7] px-3 py-2 text-sm text-[#1d1d1f]">
                {t}
              </div>
            ))}
            {(cards[c] ?? []).length === 0 && (
              <div className="rounded-xl border border-dashed border-black/15 px-3 py-4 text-center text-xs text-[#86868b]">No jobs</div>
            )}
          </div>
        </div>
      ))}
    </div>
  )
}

// The first steps on a black band, with what the page knows of them.
export function JobFirstSteps({
  steps,
  progress,
  nextId,
}: {
  steps: JourneyStep[]
  progress: { done: number; total: number }
  nextId: string | null
}) {
  return (
    <Reveal className="text-left">
      <FirstStepsPath specs={FIRST_STEPS} steps={steps} progress={progress} nextId={nextId} tone="dark" />
    </Reveal>
  )
}
