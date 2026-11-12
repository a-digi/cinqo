import { BriefcaseIcon, CameraIcon, CheckCircleIcon, LayersIcon, PDFIcon, TargetIcon, UserIcon } from '../../../Shared/Icons/icons'
import { Arrow } from '../../../Shared/Infographic/Arrow'
import { FirstStepsPath as SharedFirstStepsPath } from '../../../Shared/Infographic/FirstStepsPath'
import { FlowRow, type FlowNode } from '../../../Shared/Infographic/FlowRow'
import type { JourneyStep } from '../../../Shared/Infographic/infographicTypes'
import { BlueFrame } from '../../../Shared/Infographic/landing/BlueFrame'
import { Reveal } from '../../../Shared/Infographic/Reveal'
import { ACCENTS, accentFor, gradient, initialOf, type Accent } from '../../JobsPage/jobsTheme'
import { PROFILE_AREAS } from '../profileAreas'
import {
  COMPLETENESS_BANDS,
  COMPLETENESS_PARTS,
  EXAMPLE_ROLES,
  FIRST_STEPS,
  HERO_NODES,
  ROLE_ROWS,
  type ChapterId,
  type RoleRow,
} from './profileInfoData'
import { bandWord, partWidths, scoreOf } from './profileInfoRules'

// The pictures of the profiles infographic (plan 266). Boxes and text are real HTML (read aloud, scaling with the text size); the only SVGs are the
// tool's own small icons. Colours come from the tool's palette; the sample names and numbers are made up.

const HERO_ICONS: Record<string, React.ReactNode> = {
  profile: <UserIcon className="h-6 w-6" />,
  roles: <TargetIcon className="h-6 w-6" />,
  data: <LayersIcon className="h-6 w-6" />,
  matches: <CheckCircleIcon className="h-6 w-6" />,
  cvs: <PDFIcon className="h-6 w-6" />,
}
const HERO_ACCENTS: Accent[] = [ACCENTS[0], ACCENTS[4], ACCENTS[6], ACCENTS[5], ACCENTS[1]]

// The big picture: five nodes joined by arrows; each is a button that scrolls to its chapter.
export function HeroFlow({ onGo }: { onGo: (chapter: ChapterId) => void }) {
  const nodes: FlowNode[] = HERO_NODES.map((n, i) => ({ ...n, icon: HERO_ICONS[n.id], accent: HERO_ACCENTS[i] }))
  return (
    <FlowRow
      nodes={nodes}
      onGo={(c) => {
        onGo(c as ChapterId)
      }}
    />
  )
}

// A profile card taken apart: the name and photo, the completeness bar, platform links, the numbers of its applications and the tiles of its pages.
export function ProfileCardAnatomy() {
  const accent = ACCENTS[0]
  const name = 'Ada Lovelace'
  const tiles = [...PROFILE_AREAS.map((a) => a.label), 'Applications']
  return (
    <div
      className="mx-auto w-full max-w-2xl rounded-3xl border bg-white p-4 shadow-sm"
      style={{ borderColor: accent.from }}
      data-testid="profile-card-sample"
    >
      <Reveal delay={0}>
        <div className="flex items-center gap-3">
          <span
            className="inline-flex h-12 w-12 shrink-0 items-center justify-center rounded-full text-lg font-bold text-white"
            style={{ background: accentFor(name).to }}
            aria-hidden="true"
          >
            {initialOf(name)}
          </span>
          <div className="min-w-0 flex-1">
            <div className="text-sm font-semibold text-gray-900">{name}</div>
            <div className="text-xs text-gray-400">One job seeker</div>
          </div>
          <span className="inline-flex items-center gap-1 rounded-full border border-gray-200 px-2.5 py-1 text-xs text-gray-600">
            <CameraIcon className="h-3 w-3" /> Photo
          </span>
        </div>
      </Reveal>
      <Reveal delay={1}>
        <div className="mt-3">
          <div className="mb-1 text-xs font-semibold text-gray-700">
            Profile completeness: 70% <span className="font-normal text-gray-500">· Strong</span>
          </div>
          <div className="h-2 w-full overflow-hidden rounded-full bg-gray-200">
            <div className="h-full rounded-full" style={{ width: '70%', background: '#2563eb' }} />
          </div>
        </div>
      </Reveal>
      <Reveal delay={2}>
        <div className="mt-3 flex flex-wrap gap-2 text-xs">
          {['linkedin', 'github'].map((p) => (
            <span key={p} className="rounded-full border border-blue-200 bg-blue-50 px-2.5 py-1 font-medium text-blue-700">
              {p}
            </span>
          ))}
          <span className="rounded-full border border-gray-200 bg-white px-2.5 py-1 font-medium text-gray-700">+ Platform</span>
        </div>
      </Reveal>
      <Reveal delay={3}>
        <div className="mt-3 grid grid-cols-3 gap-2">
          {[
            ['7', 'Applications', accent],
            ['3', 'Applied', ACCENTS[1]],
            ['2', 'Waiting for a reply', ACCENTS[2]],
          ].map(([n, label, a]) => (
            <div
              key={label as string}
              className="rounded-2xl border px-3 py-2"
              style={{ borderColor: (a as Accent).soft, background: (a as Accent).soft }}
            >
              <div className="text-xl font-extrabold tabular-nums" style={{ color: (a as Accent).text }}>
                {n as string}
              </div>
              <div className="text-[11px] text-gray-600">{label as string}</div>
            </div>
          ))}
        </div>
      </Reveal>
      <Reveal delay={4}>
        <ul className="mt-3 grid grid-cols-2 gap-1.5 text-xs font-medium text-gray-700 sm:grid-cols-5">
          {tiles.map((t) => (
            <li key={t} className="rounded-xl border border-gray-100 bg-gray-50 px-2 py-1.5 text-center">
              {t}
            </li>
          ))}
        </ul>
      </Reveal>
    </div>
  )
}

const HOW_STYLE: Record<RoleRow['how'], { label: string; className: string }> = {
  follows: { label: 'Follows the profile', className: 'bg-blue-100 text-blue-800' },
  chooses: { label: 'Chooses from the library', className: 'bg-violet-100 text-violet-800' },
  own: { label: 'Its own', className: 'bg-amber-100 text-amber-800' },
  profile: { label: 'The profile’s, shared', className: 'bg-green-100 text-green-800' },
}

// One profile and the target roles it has, and a table of what a role follows, chooses or owns.
export function RolesPicture() {
  const accent = ACCENTS[4]
  return (
    <div className="mx-auto w-full max-w-3xl space-y-6">
      <div className="flex flex-col items-center gap-2 md:flex-row md:items-center">
        <Reveal className="w-full md:w-48">
          <div className="rounded-2xl border bg-white p-3 text-center shadow-sm" style={{ borderColor: ACCENTS[0].from }}>
            <UserIcon className="mx-auto h-6 w-6" />
            <div className="mt-1 text-sm font-semibold text-gray-900">Ada Lovelace</div>
            <div className="text-xs text-gray-500">the profile</div>
          </div>
        </Reveal>
        <span className="md:hidden">
          <Arrow accent={accent} down />
        </span>
        <span className="hidden md:inline-flex">
          <Arrow accent={accent} />
        </span>
        <ul className="grid w-full flex-1 gap-2">
          {EXAMPLE_ROLES.map((r, i) => (
            <Reveal as="li" key={r} delay={i + 1}>
              <div className="flex items-center gap-2 rounded-xl border bg-white px-3 py-2 shadow-sm" style={{ borderColor: accent.soft }}>
                <span
                  className="inline-flex h-7 w-7 items-center justify-center rounded-full text-white"
                  style={{ background: gradient(accent) }}
                  aria-hidden="true"
                >
                  <TargetIcon className="h-4 w-4" />
                </span>
                <span className="text-sm font-medium text-gray-900">{r}</span>
                <span className="ml-auto text-xs text-gray-400">target role</span>
              </div>
            </Reveal>
          ))}
        </ul>
      </div>
      <ul className="space-y-2" aria-label="What a target role follows, chooses or owns">
        {ROLE_ROWS.map((row, i) => (
          <Reveal as="li" key={row.id} delay={i % 3}>
            <div
              className="grid gap-2 rounded-2xl border border-gray-200 bg-white p-3 shadow-sm md:grid-cols-[11rem_13rem_1fr] md:items-center"
              data-how={row.how}
            >
              <span className="text-sm font-semibold text-gray-900">{row.what}</span>
              <span className={`inline-flex w-fit rounded-full px-2.5 py-1 text-xs font-semibold ${HOW_STYLE[row.how].className}`}>
                {HOW_STYLE[row.how].label}
              </span>
              <span className="text-xs text-gray-600">{row.note}</span>
            </div>
          </Reveal>
        ))}
      </ul>
    </div>
  )
}

const SKILLS = ['Go', 'SQL', 'Docker', 'Leadership', 'Python']
const EXPERIENCE = ['Acme, Senior Engineer', 'Globex, Team Lead', 'Initech, Developer']

// The profile's two libraries, the roles that pick from them, and the things every role shares.
export function DataPicture() {
  const accent = ACCENTS[6]
  return (
    <div className="mx-auto w-full max-w-3xl space-y-6">
      <div className="grid gap-3 md:grid-cols-[1fr_auto_1fr] md:items-stretch">
        <Reveal className="h-full">
          <div className="h-full rounded-2xl border bg-white p-4 shadow-sm" style={{ borderColor: accent.from }}>
            <div className="mb-2 text-sm font-semibold" style={{ color: accent.text }}>
              The profile&apos;s libraries
            </div>
            <div className="text-[11px] font-semibold uppercase tracking-wide text-gray-400">Skills</div>
            <div className="mt-1 flex flex-wrap gap-1.5">
              {SKILLS.map((s) => (
                <span key={s} className="rounded-full bg-sky-50 px-2.5 py-0.5 text-xs font-medium text-sky-800">
                  {s}
                </span>
              ))}
            </div>
            <div className="mt-3 text-[11px] font-semibold uppercase tracking-wide text-gray-400">Experience</div>
            <ul className="mt-1 space-y-1 text-xs text-gray-700">
              {EXPERIENCE.map((e) => (
                <li key={e} className="rounded-lg bg-gray-50 px-2 py-1">
                  {e}
                </li>
              ))}
            </ul>
          </div>
        </Reveal>
        <div aria-hidden="true" className="flex items-center justify-center">
          <span className="md:hidden">
            <Arrow accent={accent} down />
          </span>
          <span className="hidden md:inline-flex">
            <Arrow accent={accent} />
          </span>
        </div>
        <Reveal delay={1} className="h-full">
          <div className="h-full rounded-2xl border bg-white p-4 shadow-sm" style={{ borderColor: ACCENTS[4].from }}>
            <div className="mb-2 text-sm font-semibold" style={{ color: ACCENTS[4].text }}>
              A target role picks what it uses
            </div>
            <div className="text-xs font-medium text-gray-900">Backend Engineer</div>
            <div className="mt-1 flex flex-wrap gap-1.5">
              {SKILLS.slice(0, 3).map((s) => (
                <span key={s} className="rounded-full bg-violet-50 px-2.5 py-0.5 text-xs font-medium text-violet-800">
                  {s}
                </span>
              ))}
            </div>
            <ul className="mt-2 space-y-1 text-xs text-gray-700">
              {EXPERIENCE.slice(0, 2).map((e) => (
                <li key={e} className="rounded-lg bg-gray-50 px-2 py-1">
                  {e}
                </li>
              ))}
            </ul>
          </div>
        </Reveal>
      </div>
      <Reveal delay={2}>
        <div className="rounded-2xl border border-dashed border-gray-300 p-4">
          <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-400">
            The profile&apos;s own, shared by every target role
          </div>
          <ul className="grid grid-cols-2 gap-2 sm:grid-cols-5">
            {['Education', 'Languages', 'Courses', 'Hobbies', 'Photos'].map((t) => (
              <li
                key={t}
                className="rounded-xl border px-2 py-2 text-center text-xs font-semibold"
                style={{ borderColor: accent.soft, background: accent.soft, color: accent.text }}
              >
                {t}
              </li>
            ))}
          </ul>
        </div>
      </Reveal>
    </div>
  )
}

// The completeness bar made of its seven weighted parts, with a sample profile that has three of them done, and the four words.
export function CompletenessPicture() {
  const done = ['name', 'links', 'persona']
  const score = scoreOf(done, COMPLETENESS_PARTS)
  const widths = partWidths(COMPLETENESS_PARTS)
  const accent = ACCENTS[1]
  return (
    <div className="mx-auto w-full max-w-3xl space-y-5">
      <Reveal>
        <div className="rounded-2xl border bg-white p-4 shadow-sm" style={{ borderColor: accent.from }}>
          <div className="mb-2 flex items-baseline justify-between text-sm">
            <span className="font-semibold text-gray-900">A profile with a name, a link and a target role</span>
            <span data-testid="sample-score" className="font-extrabold tabular-nums" style={{ color: accent.text }}>
              {score}% · {bandWord(score, COMPLETENESS_BANDS)}
            </span>
          </div>
          <div
            className="flex h-5 w-full gap-0.5 overflow-hidden rounded-full"
            role="img"
            aria-label={`Seven parts: ${COMPLETENESS_PARTS.map((p) => `${p.label} ${String(p.weight)}`).join(', ')}`}
          >
            {COMPLETENESS_PARTS.map((p, i) => (
              <span
                key={p.id}
                data-part={p.id}
                data-done={done.includes(p.id) ? 'true' : 'false'}
                className="h-full"
                style={{ width: `${String(widths[i])}%`, background: done.includes(p.id) ? gradient(accent) : '#e5e7eb' }}
              />
            ))}
          </div>
        </div>
      </Reveal>
      <ul className="grid gap-2 sm:grid-cols-2">
        {COMPLETENESS_PARTS.map((p, i) => (
          <Reveal as="li" key={p.id} delay={i % 2}>
            <div className="flex items-center gap-3 rounded-xl border border-gray-200 bg-white px-3 py-2 shadow-sm">
              <span
                className="inline-flex h-8 min-w-[2.25rem] items-center justify-center rounded-full px-2 text-xs font-bold tabular-nums text-white"
                style={{ background: done.includes(p.id) ? gradient(accent) : '#9ca3af' }}
              >
                {p.weight}
              </span>
              <div className="min-w-0">
                <div className="text-sm font-semibold text-gray-900">{p.label}</div>
                <div className="text-xs text-gray-500">{p.needs}</div>
              </div>
            </div>
          </Reveal>
        ))}
      </ul>
      <ol className="grid grid-cols-2 gap-2 sm:grid-cols-4" aria-label="The four words">
        {COMPLETENESS_BANDS.map((b, i) => (
          <Reveal as="li" key={b.word} delay={i}>
            <div
              className="rounded-xl px-3 py-2 text-center"
              style={{ background: i === 0 ? '#fee2e2' : i === 1 ? '#fef3c7' : i === 2 ? '#dbeafe' : '#dcfce7' }}
            >
              <div className="text-sm font-semibold text-gray-900">{b.word}</div>
              <div className="text-xs text-gray-600">{i === COMPLETENESS_BANDS.length - 1 ? 'at 100' : `from ${String(b.from)}`}</div>
            </div>
          </Reveal>
        ))}
      </ol>
    </div>
  )
}

// A job scored for the profile, the auto-add rule that puts it on the profile's board, the board, and a CV made from a target role.
export function JobsPicture() {
  const steps: { id: string; title: string; body: React.ReactNode; accent: Accent }[] = [
    {
      id: 'job',
      title: 'A new job',
      accent: ACCENTS[0],
      body: (
        <>
          <div className="text-xs text-gray-700">Backend Engineer, Acme</div>
          <div className="mt-1 inline-block rounded-full bg-green-100 px-2 py-0.5 text-xs font-bold tabular-nums text-green-800">
            82% match
          </div>
        </>
      ),
    },
    {
      id: 'rule',
      title: 'An auto-add rule',
      accent: ACCENTS[4],
      body: <div className="text-xs text-gray-700">At least 70% goes to a status you choose. The first rule that fits decides.</div>,
    },
    {
      id: 'board',
      title: 'The profile’s job board',
      accent: ACCENTS[5],
      body: (
        <ul className="flex gap-1 text-[10px] font-semibold">
          {['Not Reviewed', 'Applied', 'Declined'].map((c, i) => (
            <li
              key={c}
              className="rounded-md px-1.5 py-1"
              style={{
                background: [ACCENTS[7].soft, ACCENTS[1].soft, ACCENTS[3].soft][i],
                color: [ACCENTS[7].text, ACCENTS[1].text, ACCENTS[3].text][i],
              }}
            >
              {c}
            </li>
          ))}
        </ul>
      ),
    },
    {
      id: 'cv',
      title: 'A CV',
      accent: ACCENTS[1],
      body: <div className="text-xs text-gray-700">Made from one target role, in one of a few designs.</div>,
    },
  ]
  return (
    <ol className="mx-auto flex w-full max-w-4xl flex-col items-stretch md:flex-row md:items-center" aria-label="From a job to a CV">
      {steps.map((s, i) => (
        <li key={s.id} className="contents">
          <div className="min-w-0 flex-1 md:basis-0">
            <Reveal delay={i} className="h-full">
              <div className="h-full rounded-2xl border bg-white p-3 shadow-sm" style={{ borderColor: s.accent.from }}>
                <div className="mb-1 flex items-center gap-1.5 text-xs font-semibold" style={{ color: s.accent.text }}>
                  {s.id === 'job' ? <BriefcaseIcon className="h-4 w-4" /> : s.id === 'cv' ? <PDFIcon className="h-4 w-4" /> : null}
                  {s.title}
                </div>
                {s.body}
              </div>
            </Reveal>
          </div>
          {i < steps.length - 1 && (
            <div aria-hidden="true" className="flex justify-center py-1 md:px-1 md:py-0">
              <span className="md:hidden">
                <Arrow accent={s.accent} down />
              </span>
              <span className="hidden md:inline-flex">
                <Arrow accent={s.accent} />
              </span>
            </div>
          )}
        </li>
      ))}
    </ol>
  )
}

// "Your first profile": the six steps as a numbered path with what the page can tell (done, next, or not known), and a count.
export function FirstStepsPath({
  steps,
  progress,
  nextId,
}: {
  steps: JourneyStep[]
  progress: { done: number; total: number }
  nextId: string | null
}) {
  return <SharedFirstStepsPath specs={FIRST_STEPS} steps={steps} progress={progress} nextId={nextId} />
}

// The picture of the hero band: the five parts of the flow as small blue-outlined icon tiles joined by circled "+" (a calm picture, nothing moves).
export function HeroCluster() {
  return (
    <ol aria-label="The parts of a profile" className="grid grid-cols-3 gap-x-6 gap-y-5" data-testid="hero-cluster">
      {HERO_NODES.map((n, i) => (
        <li key={n.id} className={i === 3 ? 'col-start-1 sm:col-start-auto' : ''}>
          <BlueFrame badge={i < HERO_NODES.length - 1 ? '+' : undefined} className="!bg-white/95">
            <div className="flex flex-col items-center gap-1 text-center">
              <span className="text-gray-900" aria-hidden="true">
                {HERO_ICONS[n.id]}
              </span>
              <span className="text-xs font-bold text-gray-900">{n.label}</span>
            </div>
          </BlueFrame>
        </li>
      ))}
    </ol>
  )
}

// The picture of the first tall tile of "Jobs and CVs": a job's match score, big, in a blue frame.
export function ScoreFrame() {
  return (
    <BlueFrame badge="%" className="mx-auto max-w-xs">
      <div className="text-center" data-testid="score-frame">
        <div className="text-xs font-semibold uppercase tracking-wide text-gray-400">Backend Engineer, Acme</div>
        <div className="mt-1 text-5xl font-black tabular-nums text-gray-900">82%</div>
        <div className="mx-auto mt-2 h-2 w-40 overflow-hidden rounded-full bg-gray-200">
          <div className="h-full rounded-full" style={{ width: '82%', background: '#16a34a' }} />
        </div>
        <div className="mt-1 text-xs font-medium text-green-700">match with Ada&apos;s profile</div>
      </div>
    </BlueFrame>
  )
}

// The picture of the second tall tile: an auto-add rule, "at least 70%" leading to a status you choose.
export function RuleFrame() {
  const accent = ACCENTS[4]
  return (
    <BlueFrame badge="+" className="mx-auto max-w-xs">
      <div className="flex items-center justify-center gap-2 text-center" data-testid="rule-frame">
        <div className="rounded-xl bg-violet-50 px-3 py-2">
          <div className="text-[11px] font-semibold uppercase tracking-wide text-violet-700">If the score is</div>
          <div className="text-lg font-black text-gray-900">at least 70%</div>
        </div>
        <Arrow accent={accent} />
        <div className="rounded-xl bg-green-50 px-3 py-2">
          <div className="text-[11px] font-semibold uppercase tracking-wide text-green-700">Put it on the board as</div>
          <div className="text-lg font-black text-gray-900">a status you choose</div>
        </div>
      </div>
    </BlueFrame>
  )
}
