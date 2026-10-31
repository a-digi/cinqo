import type { Persona, Profile } from '../../api'
import { PAGE_ACCENT, accentFor, initialOf } from '../JobsPage/jobsTheme'
import { groupPersonasByProfile } from './personaListRules'

// The personas as a list, grouped by profile, each a link to a page of the caller's choosing: the one shared way
// to pick a persona without a dropdown. The page supplies where an item leads (hrefFor) and what is shown under
// the name (renderExtra: a count, a few chips); the item is a real link, so it can be opened in a new tab.
export function PersonaList({
  personas,
  profiles,
  hrefFor,
  renderExtra,
}: {
  personas: Persona[]
  profiles: Profile[]
  hrefFor: (persona: Persona) => string
  renderExtra?: (persona: Persona) => React.ReactNode
}) {
  const groups = groupPersonasByProfile(personas, profiles)
  return (
    <div className="space-y-6">
      {groups.map((group) => (
        <section key={group.profileId || 'other'} aria-label={group.label}>
          <h2 className="mb-2 text-[11px] font-semibold uppercase tracking-wider text-gray-400">{group.label}</h2>
          <ul className="career-stagger space-y-2.5">
            {group.personas.map((p) => (
              <li key={p.id}>
                <a
                  href={hrefFor(p)}
                  onClick={(e) => {
                    // A plain click navigates inside the app; a modified click (new tab, window) keeps the link's own behaviour.
                    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return
                    e.preventDefault()
                    window.__cinqoToolBridge.navigate(hrefFor(p))
                  }}
                  className="flex items-start gap-3 rounded-3xl border p-4 shadow-sm transition-shadow hover:shadow-md"
                  style={{ borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }}
                >
                  <span
                    className="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-base font-bold text-white shadow-sm"
                    style={{ background: accentFor(p.name).to }}
                    aria-hidden="true"
                  >
                    {initialOf(p.name)}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block text-sm font-semibold text-gray-900">{p.name}</span>
                    {p.description && <span className="mt-0.5 block text-xs text-gray-500">{p.description}</span>}
                    {renderExtra?.(p)}
                  </span>
                </a>
              </li>
            ))}
          </ul>
        </section>
      ))}
    </div>
  )
}
