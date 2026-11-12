import type { Profile } from '../../api'
import { cardClassName, cardStyle } from '../../Shared/cardStyle'
import { PAGE_ACCENT, accentFor, initialOf } from '../JobsPage/jobsTheme'
import { profileLabel } from '../ProfileSwitcher/profileLabel'

// "Import into" (plan 261): a new profile made from the CV (the default, as before), or one of the person's profiles. Two radio cards; the second
// holds the list of profiles. Shown only when there is a profile to choose; `target` is '' for a new profile, else a profile id.
export function ImportTargetPicker({
  profiles,
  target,
  onChange,
}: {
  profiles: Profile[]
  target: string
  onChange: (profileId: string) => void
}) {
  if (profiles.length === 0) return null
  const existing = target !== ''
  const chosen = profiles.find((p) => p.id === target)
  const optionClass = (selected: boolean) =>
    `flex cursor-pointer items-start gap-3 rounded-2xl border-2 p-3.5 transition-all hover:-translate-y-0.5 hover:shadow-sm motion-reduce:transition-none motion-reduce:hover:translate-y-0 ${
      selected ? 'shadow-sm' : 'border-gray-200 bg-white'
    }`
  const selectedStyle = { borderColor: PAGE_ACCENT.from, background: PAGE_ACCENT.soft }
  return (
    <fieldset className={`career-enter mb-4 ${cardClassName}`} style={cardStyle}>
      <legend className="sr-only">Import into</legend>
      <h2 className="mb-2.5 text-sm font-semibold text-gray-900" aria-hidden="true">
        Import into
      </h2>
      <div className="grid gap-3 sm:grid-cols-2">
        {/* eslint-disable-next-line jsx-a11y/label-has-associated-control -- the text is in nested spans, which the rule does not count; the input is named by this label */}
        <label htmlFor="import-target-new" className={optionClass(!existing)} style={!existing ? selectedStyle : undefined}>
          <input
            id="import-target-new"
            type="radio"
            name="import-target"
            checked={!existing}
            onChange={() => {
              onChange('')
            }}
            className="mt-1"
          />
          <span className="min-w-0">
            <span className="block text-sm font-medium text-gray-900">A new profile from the CV</span>
            <span className="mt-0.5 block text-xs text-gray-500">The AI proposes the profile; you review it before it is saved.</span>
          </span>
        </label>
        <label htmlFor="import-target-existing" className={optionClass(existing)} style={existing ? selectedStyle : undefined}>
          <input
            id="import-target-existing"
            type="radio"
            name="import-target"
            checked={existing}
            onChange={() => {
              onChange(profiles[0].id)
            }}
            className="mt-1"
          />
          <span className="min-w-0 flex-1">
            <span className="block text-sm font-medium text-gray-900">An existing profile</span>
            <span className="mt-0.5 block text-xs text-gray-500">
              The target roles, skills, experience and education go into that profile.
            </span>
            {existing && (
              <span className="mt-2 flex items-center gap-2">
                {chosen && (
                  <span
                    aria-hidden="true"
                    className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-xs font-bold text-white"
                    style={{ background: accentFor(profileLabel(chosen)).to }}
                  >
                    {initialOf(profileLabel(chosen))}
                  </span>
                )}
                <select
                  aria-label="Profile to import into"
                  value={target}
                  onChange={(e) => {
                    onChange(e.target.value)
                  }}
                  className="min-w-0 flex-1 rounded-md border border-gray-300 bg-white px-2 py-1.5 text-sm text-gray-900"
                >
                  {profiles.map((p) => (
                    <option key={p.id} value={p.id}>
                      {profileLabel(p)}
                    </option>
                  ))}
                </select>
              </span>
            )}
          </span>
        </label>
      </div>
    </fieldset>
  )
}
