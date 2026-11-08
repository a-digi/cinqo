import type { Persona, Profile } from '../../api'
import { AppLink } from '../../Shared/AppLink/AppLink'
import { PAGE_ACCENT, accentFor, initialOf } from '../JobsPage/jobsTheme'
import { groupPersonasByProfile } from './personaListRules'

// The personas as a list, grouped by profile, each a link to a page of the caller's choosing: the one shared way
// to pick a persona without a dropdown. The page supplies where an item leads (hrefFor) and what is shown under
// the name (renderExtra: a count, a few chips); the item is a real link, so it can be opened in a new tab.
export function PersonaList({
  personas,
  profiles,
  hrefFor,
  onSelect,
  selectedId,
  renderExtra,
}: {
  personas: Persona[]
  profiles: Profile[]
  // Where an item leads: each is then a link. Leave it out and pass onSelect for a list that picks a persona.
  hrefFor?: (persona: Persona) => string
  // Picking mode: each item is a button that calls this, the one with selectedId marked.
  onSelect?: (persona: Persona) => void
  selectedId?: string | null
  renderExtra?: (persona: Persona) => React.ReactNode
}) {
  const groups = groupPersonasByProfile(personas, profiles)
  const itemClass = 'flex items-start gap-3 rounded-3xl border p-4 shadow-sm transition-shadow hover:shadow-md'
  const itemStyle = { borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }
  return (
    <div className="space-y-6">
      {groups.map((group) => (
        <section key={group.profileId || 'other'} aria-label={group.label}>
          <h2 className="mb-2 text-[11px] font-semibold uppercase tracking-wider text-gray-400">{group.label}</h2>
          <ul className="career-stagger space-y-2.5">
            {group.personas.map((p) => (
              <li key={p.id}>
                {onSelect ? (
                  <button
                    type="button"
                    aria-pressed={selectedId === p.id}
                    onClick={() => {
                      onSelect(p)
                    }}
                    className={`${itemClass} w-full text-left ${selectedId === p.id ? 'ring-2 ring-purple-400' : ''}`}
                    style={itemStyle}
                  >
                    <ItemBody persona={p} extra={renderExtra?.(p)} />
                  </button>
                ) : (
                  <AppLink href={hrefFor?.(p) ?? ''} className={itemClass} style={itemStyle}>
                    <ItemBody persona={p} extra={renderExtra?.(p)} />
                  </AppLink>
                )}
              </li>
            ))}
          </ul>
        </section>
      ))}
    </div>
  )
}

// The inside of an item: the round initial, the name, the description and what the page adds under them.
function ItemBody({ persona, extra }: { persona: Persona; extra: React.ReactNode }) {
  return (
    <>
      <span
        className="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-base font-bold text-white shadow-sm"
        style={{ background: accentFor(persona.name).to }}
        aria-hidden="true"
      >
        {initialOf(persona.name)}
      </span>
      <span className="min-w-0 flex-1">
        <span className="block text-sm font-semibold text-gray-900">{persona.name}</span>
        {persona.description && <span className="mt-0.5 block text-xs text-gray-500">{persona.description}</span>}
        {extra}
      </span>
    </>
  )
}
