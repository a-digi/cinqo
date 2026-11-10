import { useEffect, useState } from 'react'
import {
  fetchPersonaDetails,
  fetchProfileDetails,
  updatePersonaDetails,
  updateProfileDetails,
  type PersonaDetails,
  type Persona,
} from '../../api'
import { TargetIcon } from '../../Shared/Icons/icons'
import { PageTips } from '../../Shared/PageTips/PageTips'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { GOALS_PATH } from '../../paths'
import { PAGE_ACCENT } from '../JobsPage/jobsTheme'
import { ProfileBar } from '../ProfileLibrary/ProfileBar'
import { personasOfProfile } from '../ProfileLibrary/libraryRules'
import { useProfileChoice } from '../ProfileLibrary/useProfileChoice'
import { BackToProfile } from '../ProfileItems/BackToProfile'
import { GoalsFields } from './GoalsFields'
import {
  followRequest,
  goalsFormOf,
  goalsFormOfProfile,
  goalsPatch,
  goalsProblem,
  hasChanges,
  isOwn,
  type GoalKey,
  type GoalsForm,
} from './goalsRules'

const cardStyle = { borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }

function errorText(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

// The Goals page (plan 174): what the profile is looking for (desired titles and locations, minimum salary, remote work) is the
// PROFILE's, edited once at the top; below, each persona of the profile shows what it uses, marked "Following the profile" or "Own
// wording", and can word a goal itself or go back to the profile's.
export function GoalsPage() {
  const { profiles, personas, profileId, select, error: loadError, loaded } = useProfileChoice(GOALS_PATH)
  const mine = personasOfProfile(personas, profileId)
  // Bumped after the profile's goals are saved: the persona cards below read again (the ones that follow show the new values).
  const [version, setVersion] = useState(0)

  return (
    <div className="career-v-zoom career-a-float career-t-type max-w-5xl p-6 font-sans text-gray-900">
      {profileId !== '' && <BackToProfile profileId={profileId} profiles={profiles} />}
      <PageHeaderCard
        icon={<TargetIcon />}
        title="Goals"
        help={GOALS_PATH}
        description="What the profile is looking for. It is set once for the profile; every target role uses it unless the target role has its own. The AI reads it when matching jobs and writing CVs."
      />
      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <div className="order-1 min-w-0 flex-1 lg:order-2">
          {loadError !== '' && <p className="mb-3 text-sm text-red-700">{loadError}</p>}
          {loaded && loadError === '' && profiles.length === 0 && (
            <p className="rounded-3xl border border-gray-100 bg-gray-50 p-8 text-center text-sm text-gray-500">
              There is no profile yet. Create one on the Profiles page first.
            </p>
          )}
          {profileId !== '' && (
            <>
              <ProfileBar
                profiles={profiles}
                personas={[]}
                profileId={profileId}
                onSelect={select}
                personaHref={() => GOALS_PATH}
                personaLabel=""
              />
              <ProfileGoals
                key={profileId}
                profileId={profileId}
                onSaved={() => {
                  setVersion((v) => v + 1)
                }}
              />
              <h2 className="mb-2 mt-6 text-sm font-semibold text-gray-700">Target roles</h2>
              {mine.length === 0 && <p className="text-sm text-gray-500">This profile has no target roles yet.</p>}
              <div className="space-y-4">
                {mine.map((p) => (
                  <PersonaGoals key={`${p.id}-${String(version)}`} persona={p} />
                ))}
              </div>
            </>
          )}
        </div>
        <aside className="order-2 w-full shrink-0 lg:order-1 lg:sticky lg:top-6 lg:w-72">
          <PageTips page="goals" orientation="vertical" />
        </aside>
      </div>
    </div>
  )
}

function ProfileGoals({ profileId, onSaved }: { profileId: string; onSaved: () => void }) {
  const [form, setForm] = useState<GoalsForm | null>(null)
  const [saved, setSaved] = useState<GoalsForm | null>(null)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [justSaved, setJustSaved] = useState(false)

  useEffect(() => {
    fetchProfileDetails(profileId)
      .then((d) => {
        setForm(goalsFormOfProfile(d))
        setSaved(goalsFormOfProfile(d))
      })
      .catch((err: unknown) => {
        setError(errorText(err))
      })
  }, [profileId])

  if (!form || !saved) return <p className="text-sm text-gray-500">{error !== '' ? error : 'Loading…'}</p>

  function handleSave() {
    if (!form || !saved) return
    const problem = goalsProblem(form)
    if (problem !== '') {
      setError(problem)
      return
    }
    const patch = goalsPatch(form, saved)
    if (!hasChanges(patch)) return
    setError('')
    setSaving(true)
    updateProfileDetails(profileId, patch)
      .then((d) => {
        setForm(goalsFormOfProfile(d))
        setSaved(goalsFormOfProfile(d))
        setJustSaved(true)
        onSaved()
      })
      .catch((err: unknown) => {
        setError(errorText(err))
      })
      .finally(() => {
        setSaving(false)
      })
  }

  return (
    <section className="career-enter career-delay-1 rounded-3xl border p-5 shadow-sm" style={cardStyle}>
      <h2 className="mb-1 text-sm font-semibold text-gray-700">Goals of the profile</h2>
      <p className="mb-3 text-xs text-gray-500">Every target role below uses these unless it has its own.</p>
      <GoalsFields
        idPrefix="profile-goals"
        form={form}
        onChange={(patch) => {
          setJustSaved(false)
          setForm({ ...form, ...patch })
        }}
      />
      <div className="min-h-[1.2em] pt-3 text-sm text-red-700">{error}</div>
      <div className="mt-2 flex items-center gap-3">
        <button
          type="button"
          onClick={handleSave}
          disabled={saving}
          className="rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800 disabled:opacity-50"
        >
          {saving ? 'Saving…' : 'Save goals'}
        </button>
        {justSaved && <span className="text-sm text-green-700">Saved</span>}
      </div>
    </section>
  )
}

function PersonaGoals({ persona }: { persona: Persona }) {
  const [details, setDetails] = useState<PersonaDetails | null>(null)
  const [form, setForm] = useState<GoalsForm | null>(null)
  const [saved, setSaved] = useState<GoalsForm | null>(null)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [justSaved, setJustSaved] = useState(false)

  function adopt(d: PersonaDetails | null) {
    setDetails(d)
    setForm(goalsFormOf(d))
    setSaved(goalsFormOf(d))
  }

  useEffect(() => {
    fetchPersonaDetails(persona.id)
      .then((r) => {
        adopt(r.personaDetails)
      })
      .catch((err: unknown) => {
        setError(errorText(err))
      })
  }, [persona.id])

  if (!form || !saved) return <p className="text-sm text-gray-500">{error !== '' ? error : `Loading ${persona.name}…`}</p>

  const own = (key: GoalKey) => isOwn(details?.overridden, key)

  function handleSave() {
    if (!form || !saved) return
    const problem = goalsProblem(form)
    if (problem !== '') {
      setError(problem)
      return
    }
    const patch = goalsPatch(form, saved)
    if (!hasChanges(patch)) return
    setError('')
    setSaving(true)
    updatePersonaDetails(persona.id, patch)
      .then((r) => {
        adopt(r.personaDetails)
        setJustSaved(true)
      })
      .catch((err: unknown) => {
        setError(errorText(err))
      })
      .finally(() => {
        setSaving(false)
      })
  }

  function handleFollow(key: GoalKey) {
    setError('')
    updatePersonaDetails(persona.id, followRequest(own(key), key))
      .then((r) => {
        adopt(r.personaDetails)
        setJustSaved(false)
      })
      .catch((err: unknown) => {
        setError(errorText(err))
      })
  }

  return (
    <section className="rounded-3xl border bg-white p-5 shadow-sm" style={{ borderColor: PAGE_ACCENT.soft }}>
      <h3 className="mb-3 text-sm font-semibold text-gray-800">{persona.name}</h3>
      <GoalsFields
        idPrefix={`persona-${persona.id}`}
        form={form}
        onChange={(patch) => {
          setJustSaved(false)
          setForm({ ...form, ...patch })
        }}
        own={own}
        onFollow={handleFollow}
      />
      <div className="min-h-[1.2em] pt-3 text-sm text-red-700">{error}</div>
      <div className="mt-2 flex items-center gap-3">
        <button
          type="button"
          onClick={handleSave}
          disabled={saving}
          className="rounded-md bg-gray-800 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-900 disabled:opacity-50"
        >
          {saving ? 'Saving…' : 'Save for this target role'}
        </button>
        {justSaved && <span className="text-sm text-green-700">Saved</span>}
      </div>
    </section>
  )
}
