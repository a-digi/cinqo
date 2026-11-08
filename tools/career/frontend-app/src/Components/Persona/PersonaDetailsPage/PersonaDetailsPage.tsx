import { useEffect, useState } from 'react'
import { fetchPersonaDetails, fetchPersonas, fetchProfiles, updatePersonaDetails, type Persona, type Profile } from '../../../api'
import { TargetIcon } from '../../../Shared/Icons/icons'
import { inputClass } from '../../../Shared/inputClass'
import { PageHeaderCard } from '../../../Shared/PageHeader/PageHeaderCard'
import { GOALS_PATH, PERSONA_DETAILS_PATH, experiencePathFor, skillsPathFor } from '../../../paths'
import { Field } from '../../Field/Field'
import { PAGE_ACCENT } from '../../JobsPage/jobsTheme'
import { profileLabel } from '../../ProfileSwitcher/profileLabel'
import { FieldStepper } from './FieldStepper'
import { positioningCompleteness, positioningSteps, type DetailsKey } from './detailsStats'
import { FOLLOWING_TEXT, OWN_TEXT, followRequest, isOwn, summaryPatch } from '../../GoalsPage/goalsRules'
import { PageTips } from '../../../Shared/PageTips/PageTips'

const emptyForm: { headline: string; summary: string; location: string } = { headline: '', summary: '', location: '' }

const cardStyle = { borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }

// Renamed from ProfilePage.tsx as of step 11, matching the backend's
// own step 10 rename: this was never the job seeker's own profile
// (that's the new ProfilesPage/Profile entity) — it's a persona's own
// career positioning (headline/summary/location/desired titles/min
// salary). The overview on the left shows how complete the saved details are,
// next to the persona's skill and experience counts (the same response). See
// plan/ai/tools/career/step-07-dedicated-skills-and-experience-pages.md,
// plan/ai/tools/career/step-09-persona-frontend.md, and
// plan/ai/tools/career/step-11-job-seeker-profile-frontend.md.
export function PersonaDetailsPage({ personaId }: { personaId: string }) {
  // The persona comes from the address (/tools/career/persona-details/:personaId), not from a dropdown.
  const [persona, setPersona] = useState<Persona | null>(null)
  const [profile, setProfile] = useState<Profile | null>(null)
  const [notFound, setNotFound] = useState('')
  const [personaLoaded, setPersonaLoaded] = useState(false)
  const [form, setForm] = useState(emptyForm)
  // What is saved, so the overview does not move while a field is being typed.
  const [saved, setSaved] = useState(emptyForm)
  // Whether the persona words its summary itself (else it shows the profile's).
  const [summaryOwn, setSummaryOwn] = useState(false)
  const [skillCount, setSkillCount] = useState(0)
  const [experienceCount, setExperienceCount] = useState(0)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [justSaved, setJustSaved] = useState(false)

  function load(forPersonaId: string) {
    setError('')
    setJustSaved(false)
    fetchPersonaDetails(forPersonaId)
      .then((result) => {
        const loaded = {
          headline: result.personaDetails?.headline ?? '',
          summary: result.personaDetails?.summary ?? '',
          location: result.personaDetails?.location ?? '',
        }
        setForm(loaded)
        setSaved(loaded)
        setSummaryOwn(isOwn(result.personaDetails?.overridden, 'summary'))
        setSkillCount(result.skills.length)
        setExperienceCount(result.experience.length)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  useEffect(() => {
    setPersona(null)
    setProfile(null)
    setNotFound('')
    setPersonaLoaded(false)
    fetchPersonas()
      .then((list) => {
        const found = list.find((p) => p.id === personaId)
        if (!found) {
          setNotFound('Persona not found')
          return
        }
        setPersona(found)
        load(found.id)
        fetchProfiles()
          .then((profiles) => {
            setProfile(profiles.find((p) => p.id === found.profileId) ?? null)
          })
          .catch(() => {
            setProfile(null)
          })
      })
      .catch((err: unknown) => {
        setNotFound(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setPersonaLoaded(true)
      })
  }, [personaId])

  function handleSave() {
    setError('')
    setJustSaved(false)
    setSaving(true)
    // The summary is sent only when it was changed: an untouched one keeps following the profile.
    updatePersonaDetails(personaId, { headline: form.headline, location: form.location, ...summaryPatch(form.summary, saved.summary) })
      .then((result) => {
        setSaved(form)
        setSummaryOwn(isOwn(result.personaDetails?.overridden, 'summary'))
        setJustSaved(true)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setSaving(false)
      })
  }

  function edit(patch: Partial<typeof emptyForm>) {
    setJustSaved(false)
    setForm({ ...form, ...patch })
  }

  const completeness = positioningCompleteness(saved)

  function handleFollowSummary() {
    setError('')
    updatePersonaDetails(personaId, followRequest(summaryOwn, 'summary'))
      .then(() => {
        load(personaId)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  // The stepper's click: bring that field into view and put the cursor in it.
  function focusField(key: DetailsKey) {
    const el = document.getElementById(`details-${key}`)
    el?.scrollIntoView({ behavior: 'smooth', block: 'center' })
    el?.focus({ preventScroll: true })
  }

  const backLink = (
    <a
      href={PERSONA_DETAILS_PATH}
      onClick={(e) => {
        e.preventDefault()
        window.__cinqoToolBridge.navigate(PERSONA_DETAILS_PATH)
      }}
      className="mb-3 inline-block text-sm font-medium text-blue-700 hover:underline"
    >
      ← All summaries
    </a>
  )

  if (!persona) {
    return (
      <div className="max-w-3xl p-6 font-sans text-gray-900">
        {backLink}
        <div className="text-sm text-gray-500">{personaLoaded ? notFound : 'Loading…'}</div>
      </div>
    )
  }

  return (
    <div className="career-v-wipe career-a-float career-t-underline max-w-7xl p-6 font-sans text-gray-900">
      {backLink}
      <PageHeaderCard
        icon={<TargetIcon />}
        title={`Summary: ${persona.name}`}
        description={`${profile ? `${profileLabel(profile)} · ` : ''}How this persona presents itself. The summary starts from the profile's and can be worded for this persona. The AI reads it when matching jobs and writing CVs; nothing here is a secret.`}
        pill={{ text: `${completeness.percent}% complete` }}
      />
      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <div className="order-1 min-w-0 flex-1 lg:order-2">
          <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

          {
            <section className="career-enter career-delay-1 rounded-3xl border p-5 shadow-sm" style={cardStyle}>
              <h2 className="mb-3 text-sm font-semibold text-gray-700">Positioning</h2>
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <Field
                  label="Headline"
                  id="details-headline"
                  value={form.headline}
                  onChange={(v) => {
                    edit({ headline: v })
                  }}
                />
                <Field
                  label="Location"
                  id="details-location"
                  value={form.location}
                  onChange={(v) => {
                    edit({ location: v })
                  }}
                />
              </div>
              <label className="mt-3 block text-xs font-medium text-gray-500">
                Summary
                <textarea
                  id="details-summary"
                  value={form.summary}
                  onChange={(e) => {
                    edit({ summary: e.target.value })
                  }}
                  rows={4}
                  className={`mt-1 ${inputClass}`}
                />
              </label>

              <div className="mt-1.5 flex flex-wrap items-center gap-2 text-[11px]">
                <span
                  className={
                    summaryOwn
                      ? 'rounded-full bg-amber-50 px-2 py-0.5 font-medium text-amber-700'
                      : 'rounded-full bg-gray-100 px-2 py-0.5 text-gray-500'
                  }
                >
                  {summaryOwn ? OWN_TEXT : FOLLOWING_TEXT}
                </span>
                {summaryOwn && (
                  <button type="button" onClick={handleFollowSummary} className="text-blue-700 hover:underline">
                    Use the profile’s summary
                  </button>
                )}
                <span className="text-gray-400">
                  The profile’s summary is edited on the{' '}
                  <a
                    href={PERSONA_DETAILS_PATH}
                    onClick={(e) => {
                      e.preventDefault()
                      window.__cinqoToolBridge.navigate(PERSONA_DETAILS_PATH)
                    }}
                    className="text-blue-700 hover:underline"
                  >
                    Summary page
                  </a>
                  ; the goals on the{' '}
                  <a
                    href={GOALS_PATH}
                    onClick={(e) => {
                      e.preventDefault()
                      window.__cinqoToolBridge.navigate(GOALS_PATH)
                    }}
                    className="text-blue-700 hover:underline"
                  >
                    Goals page
                  </a>
                  .
                </span>
              </div>

              <div className="mt-5 flex items-center gap-3">
                <button
                  type="button"
                  onClick={handleSave}
                  disabled={saving}
                  className="rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800 disabled:opacity-50"
                >
                  {saving ? 'Saving…' : 'Save'}
                </button>
                {justSaved && <span className="text-sm text-green-700">Saved</span>}
              </div>
            </section>
          }
        </div>

        <aside className="order-2 w-full shrink-0 lg:order-1 career-rail-sticky lg:w-72">
          {
            <>
              <FieldStepper
                steps={positioningSteps(saved)}
                skillCount={skillCount}
                experienceCount={experienceCount}
                onPick={focusField}
                onOpenSkills={() => {
                  window.__cinqoToolBridge.navigate(skillsPathFor(personaId))
                }}
                onOpenExperience={() => {
                  window.__cinqoToolBridge.navigate(experiencePathFor(personaId))
                }}
              />
            </>
          }
          <div className="mt-8">
            <PageTips page="summary" orientation="vertical" />
          </div>
        </aside>
      </div>
    </div>
  )
}
