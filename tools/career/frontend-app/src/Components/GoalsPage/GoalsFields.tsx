import { Field } from '../Field/Field'
import { REMOTE_OPTIONS } from '../Persona/PersonaDetailsPage/remoteRules'
import { FOLLOWING_TEXT, OWN_TEXT, type GoalKey, type GoalsForm } from './goalsRules'

// The four goals of one set: the profile's, or one persona's. For a persona each goal says whether it is the persona's own wording or
// follows the profile, and an own one has a button that returns it to following (follow = undefined: the profile's own set, no marks).
export function GoalsFields({
  idPrefix,
  form,
  onChange,
  own,
  onFollow,
}: {
  idPrefix: string
  form: GoalsForm
  onChange: (patch: Partial<GoalsForm>) => void
  own?: (key: GoalKey) => boolean
  onFollow?: (key: GoalKey) => void
}) {
  const mark = (key: GoalKey) => {
    if (!own) return null
    const isOwn = own(key)
    return (
      <span className="mt-1 flex items-center gap-2 text-[11px]">
        <span
          className={
            isOwn ? 'rounded-full bg-amber-50 px-2 py-0.5 font-medium text-amber-700' : 'rounded-full bg-gray-100 px-2 py-0.5 text-gray-500'
          }
        >
          {isOwn ? OWN_TEXT : FOLLOWING_TEXT}
        </span>
        {isOwn && onFollow && (
          <button
            type="button"
            onClick={() => {
              onFollow(key)
            }}
            className="text-blue-700 hover:underline"
          >
            Use the profile’s value
          </button>
        )}
      </span>
    )
  }
  return (
    <>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <div>
          <Field
            label="Desired titles"
            id={`${idPrefix}-desiredTitles`}
            value={form.desiredTitles}
            onChange={(v) => {
              onChange({ desiredTitles: v })
            }}
          />
          {mark('desiredTitles')}
        </div>
        <div>
          <Field
            label="Desired locations"
            id={`${idPrefix}-desiredLocations`}
            value={form.desiredLocations}
            onChange={(v) => {
              onChange({ desiredLocations: v })
            }}
          />
          {mark('desiredLocations')}
        </div>
        <div>
          <Field
            label="Minimum salary"
            id={`${idPrefix}-minSalary`}
            type="number"
            value={form.minSalary}
            onChange={(v) => {
              onChange({ minSalary: v })
            }}
          />
          {mark('minSalary')}
        </div>
      </div>

      <fieldset className="mt-4">
        <legend className="text-xs font-medium text-gray-500">Remote work</legend>
        <p className="mb-1.5 mt-0.5 text-xs text-gray-400">Whether remote work is wanted. The job match uses it to judge a job’s place.</p>
        <div className="space-y-1.5">
          {REMOTE_OPTIONS.map((o) => {
            const id = `${idPrefix}-remote-${o.value === '' ? 'unset' : o.value}`
            return (
              <div key={o.value} className="flex items-start gap-2">
                <input
                  id={id}
                  type="radio"
                  name={`${idPrefix}-remote-preference`}
                  value={o.value}
                  checked={form.remotePreference === o.value}
                  onChange={() => {
                    onChange({ remotePreference: o.value })
                  }}
                  className="mt-1"
                />
                <div>
                  <label htmlFor={id} className="cursor-pointer text-sm font-medium text-gray-800">
                    {o.label}
                  </label>
                  <p className="text-xs text-gray-500">{o.hint}</p>
                </div>
              </div>
            )
          })}
        </div>
        {mark('remotePreference')}
      </fieldset>
    </>
  )
}
