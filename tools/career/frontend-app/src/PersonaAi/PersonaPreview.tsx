import type { ProposedPersona } from '../api'
import { PAGE_ACCENT, accentFor, initialOf } from '../Components/JobsPage/jobsTheme'
import { SparkleIcon } from '../Shared/Icons/icons'
import { isNewCompany, isNewSkill, isNewTitle, newItems, newItemsText, type KnownData } from './knownData'
import { salaryText } from './reviewRules'

// A read-only view of a persona as it will be saved (step 113): the persona card as the Personas
// page shows it, then its summary and goals, skills and experience. The persona given is the final
// content (finalPersona), so what is shown is what will exist.
export function PersonaPreview({ persona, known }: { persona: ProposedPersona; known?: KnownData | null }) {
  const d = persona.details
  // Items that are not in the user's own data, marked amber (step 113); nothing without a comparison.
  const notInData = known ? newItemsText(newItems(known, persona)) : null
  const badge = <span className="ml-1.5 rounded bg-amber-100 px-1.5 py-0.5 text-[10px] font-medium text-amber-800">not in your data</span>
  const field = (label: string, value: string) => (
    <div className="min-w-0">
      <dt className="text-[11px] font-medium uppercase tracking-wide text-gray-400">{label}</dt>
      <dd className="mt-0.5 break-words text-sm text-gray-800">{value === '' ? <span className="text-gray-400">Not set</span> : value}</dd>
    </div>
  )
  return (
    <article
      aria-label={`Preview of ${persona.name}`}
      className="overflow-hidden rounded-3xl border shadow-sm"
      style={{ borderColor: '#d8b4fe', background: 'linear-gradient(160deg, #ffffff 30%, #f3e8ff 100%)' }}
    >
      <div className="flex items-start gap-3 p-4">
        <span
          className="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-base font-bold text-white shadow-sm"
          style={{ background: accentFor(persona.name).to }}
          aria-hidden="true"
        >
          {initialOf(persona.name)}
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-sm font-semibold">{persona.name}</span>
            <span className="inline-flex items-center gap-1 rounded-full bg-purple-50 px-2 py-0.5 text-[11px] font-medium text-purple-700">
              <SparkleIcon />
              Created with AI
            </span>
          </div>
          {persona.description !== '' && <div className="mt-0.5 text-xs text-gray-500">{persona.description}</div>}
        </div>
      </div>

      {notInData && (
        <p className="border-t border-amber-200 bg-amber-50 px-4 py-2 text-xs text-amber-900" style={{ borderColor: '#fde68a' }}>
          {notInData} Check these before you save.
        </p>
      )}
      <div className="space-y-4 border-t bg-white/70 p-4" style={{ borderColor: PAGE_ACCENT.soft }}>
        <section aria-label="Summary and goals">
          <h3 className="mb-2 text-xs font-semibold text-gray-700">Summary &amp; Goals</h3>
          <dl className="grid gap-3 sm:grid-cols-2">
            {field('Headline', d.headline)}
            {field('Location', d.location)}
            {field('Desired titles', d.desiredTitles)}
            {field('Desired locations', d.desiredLocations)}
            {field('Minimum salary', salaryText(d.minSalary))}
          </dl>
          <p className="mt-3 whitespace-pre-wrap break-words text-sm text-gray-800">
            {d.summary === '' ? <span className="text-gray-400">No summary.</span> : d.summary}
          </p>
        </section>

        <section aria-label="Skills">
          <h3 className="mb-2 text-xs font-semibold text-gray-700">Skills</h3>
          {persona.skills.length === 0 ? (
            <p className="text-sm text-gray-400">No skills.</p>
          ) : (
            <ul className="flex flex-wrap gap-1.5">
              {persona.skills.map((s) => (
                <li
                  key={s}
                  title={known && isNewSkill(known, s) ? 'Not in your data' : undefined}
                  className={`rounded-full px-2.5 py-1 text-xs ${known && isNewSkill(known, s) ? 'bg-amber-100 text-amber-900 ring-1 ring-amber-300' : ''}`}
                  style={known && isNewSkill(known, s) ? undefined : { background: PAGE_ACCENT.soft, color: PAGE_ACCENT.text }}
                >
                  {s}
                  {known && isNewSkill(known, s) && <span className="ml-1 text-[10px] font-medium">not in your data</span>}
                </li>
              ))}
            </ul>
          )}
        </section>

        <section aria-label="Experience">
          <h3 className="mb-2 text-xs font-semibold text-gray-700">Experience</h3>
          {persona.experience.length === 0 ? (
            <p className="text-sm text-gray-400">No experience entries.</p>
          ) : (
            <ul className="space-y-2.5">
              {persona.experience.map((r, i) => (
                <li key={i} className="rounded-xl border border-gray-100 bg-white p-3">
                  <p className="text-sm font-medium text-gray-900">
                    {r.title}
                    {known && isNewTitle(known, r.title) && badge} <span className="font-normal text-gray-500">at {r.company}</span>
                    {known && isNewCompany(known, r.company) && badge}
                  </p>
                  {(r.startDate !== '' || r.endDate !== '') && (
                    <p className="text-xs text-gray-400">
                      {r.startDate === '' ? '…' : r.startDate} to {r.endDate === '' ? 'present' : r.endDate}
                    </p>
                  )}
                  {r.description !== '' && <p className="mt-1 whitespace-pre-wrap break-words text-xs text-gray-600">{r.description}</p>}
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>
    </article>
  )
}
