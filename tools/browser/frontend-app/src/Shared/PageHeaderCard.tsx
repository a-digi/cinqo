import { PAGE_ACCENT, gradient, type Accent } from './theme'

// A copy of the Career tool's PageHeaderCard: a tool's build is self-contained, so the Browser tool
// keeps its own small copy of the shared look (see tools/career/frontend-app/src/Shared/PageHeader/PageHeaderCard.tsx).
// The card at the top of a Browser page: a gradient icon badge, the title, a one
// line description and a count pill on the right, over a softly tinted background with
// a few decorative shapes. The same look the Career pages have, so the pages match.
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
      className="browser-enter relative mb-5 overflow-hidden rounded-3xl border p-5"
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
          className="inline-flex h-12 w-12 shrink-0 items-center justify-center rounded-2xl text-white shadow-sm"
          style={{ background: gradient(accent) }}
        >
          {icon}
        </span>
        <div className="min-w-0 flex-1">
          <h1 className="text-xl font-semibold">{title}</h1>
          <p className="mt-0.5 text-sm text-gray-500">{description}</p>
        </div>
        {pill && (
          <span
            className="shrink-0 rounded-full px-3 py-1 text-sm font-semibold"
            style={{ background: pillAccent.soft, color: pillAccent.text }}
          >
            {pill.text}
          </span>
        )}
      </div>
    </div>
  )
}
