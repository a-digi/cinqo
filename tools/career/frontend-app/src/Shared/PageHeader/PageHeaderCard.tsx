import { PAGE_ACCENT, gradient, type Accent } from '../../Components/JobsPage/jobsTheme'

// The card at the top of a Career list page: a gradient icon badge, the title, a one
// line description and a count pill on the right, over a softly tinted background with
// a few decorative shapes. The same look the Jobs page has, so the pages match.
export function PageHeaderCard({
  icon,
  title,
  description,
  pill,
  accent = PAGE_ACCENT,
}: {
  icon: React.ReactNode
  title: string
  description: string
  // The small pill on the right ("42 jobs", "3 running"); omit for none.
  pill?: { text: string; accent?: Accent }
  accent?: Accent
}) {
  const pillAccent = pill?.accent ?? accent
  return (
    <div
      className="career-enter career-hd relative mb-5 overflow-hidden rounded-3xl border p-5"
      style={{ borderColor: accent.soft, background: `linear-gradient(160deg, #ffffff 40%, ${accent.soft} 100%)` }}
    >
      <div
        className="pointer-events-none absolute -right-6 -top-8 h-32 w-32 rounded-full opacity-15"
        style={{ background: gradient(accent) }}
      />
      <div
        className="pointer-events-none absolute right-24 top-10 h-10 w-10 rotate-45 rounded-xl opacity-20"
        style={{ background: gradient(accent) }}
      />
      <div className="relative flex items-center gap-4">
        <span
          className="career-hd-icon inline-flex h-12 w-12 shrink-0 items-center justify-center rounded-2xl text-white shadow-sm"
          style={{ background: gradient(accent) }}
        >
          {icon}
        </span>
        <div className="min-w-0 flex-1">
          <h1 className="career-hd-title text-xl font-semibold" aria-label={title}>
            <TitleLetters text={title} />
          </h1>
          <p className="mt-0.5 text-sm text-gray-500">{description}</p>
        </div>
        {pill && (
          <span
            className="career-hd-pill shrink-0 rounded-full px-3 py-1 text-sm font-semibold"
            style={{ background: pillAccent.soft, color: pillAccent.text }}
          >
            {pill.text}
          </span>
        )}
      </div>
    </div>
  )
}

// The title split into letters, so a page can type it out or run a wave through it (the classes
// career-t-type and career-t-wave on the page's root switch that on; without one the letters just sit
// there). Words stay whole, so a long title still wraps between words; the screen reader reads the
// heading's aria-label, not the pieces.
function TitleLetters({ text }: { text: string }) {
  let index = 0
  return (
    <span aria-hidden="true">
      {text.split(' ').map((word, w) => (
        <span key={w}>
          {w > 0 && ' '}
          <span className="inline-block whitespace-nowrap">
            {Array.from(word).map((ch) => (
              <span key={index} className="career-ch" style={{ '--i': index++ } as React.CSSProperties}>
                {ch}
              </span>
            ))}
          </span>
        </span>
      ))}
    </span>
  )
}
