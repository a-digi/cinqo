import type { Persona, Profile } from '../../api'
import { AppLink } from '../../Shared/AppLink/AppLink'
import { PAGE_ACCENT, accentFor, initialOf } from '../JobsPage/jobsTheme'
import { groupPersonasByProfile, type PersonaGroup } from './personaListRules'

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
  renderGroupExtra,
  renderAction,
}: {
  personas: Persona[]
  profiles: Profile[]
  // Where an item leads: each is then a link. Leave it out and pass onSelect for a list that picks a persona.
  hrefFor?: (persona: Persona) => string
  // Picking mode: each item is a button that calls this, the one with selectedId marked.
  onSelect?: (persona: Persona) => void
  selectedId?: string | null
  renderExtra?: (persona: Persona) => React.ReactNode
  // What sits at the right end of a profile's heading (a count); nothing when left out.
  renderGroupExtra?: (group: PersonaGroup<Persona>) => React.ReactNode
  // A button (or anything interactive) at the right of an item, beside the link or the picking button and never inside it (a button inside
  // a link is invalid and a click would also open the link). Leave it out and the item is the plain link or button as before. Plan 260.
  renderAction?: (persona: Persona) => React.ReactNode
}) {
  const groups = groupPersonasByProfile(personas, profiles)
  const boxClass = 'gap-3 rounded-3xl border p-4 shadow-sm transition-shadow hover:shadow-md'
  const itemClass = `flex items-start ${boxClass}`
  const itemStyle = { borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }
  return (
    <div className="space-y-6">
      {groups.map((group) => (
        <section key={group.profileId || 'other'} aria-label={group.label}>
          <div className="mb-2 flex items-center justify-between gap-3">
            <h2 className="text-[11px] font-semibold uppercase tracking-wider text-gray-400">{group.label}</h2>
            {renderGroupExtra?.(group)}
          </div>
          <ul className="career-stagger space-y-2.5">
            {group.personas.map((p) => (
              <li key={p.id}>
                {renderAction ? (
                  // The card is a container; the link (or picking button) and the action are its two children.
                  <div
                    className={`flex flex-wrap items-center ${boxClass} ${onSelect && selectedId === p.id ? 'ring-2 ring-purple-400' : ''}`}
                    style={itemStyle}
                  >
                    {onSelect ? (
                      <button
                        type="button"
                        aria-pressed={selectedId === p.id}
                        onClick={() => {
                          onSelect(p)
                        }}
                        className="flex min-w-[14rem] flex-1 items-start gap-3 text-left"
                      >
                        <ItemBody persona={p} extra={renderExtra?.(p)} />
                      </button>
                    ) : (
                      <AppLink href={hrefFor?.(p) ?? ''} className="flex min-w-[14rem] flex-1 items-start gap-3">
                        <ItemBody persona={p} extra={renderExtra?.(p)} />
                      </AppLink>
                    )}
                    <div className="shrink-0">{renderAction(p)}</div>
                  </div>
                ) : onSelect ? (
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
