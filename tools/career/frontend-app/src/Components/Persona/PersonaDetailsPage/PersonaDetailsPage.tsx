import { useEffect, useState } from 'react'
import { fetchPersonaDetails, updatePersonaDetails } from '../../../api'
import { TargetIcon } from '../../../Shared/Icons/icons'
import { inputClass } from '../../../Shared/inputClass'
import { PageHeaderCard } from '../../../Shared/PageHeader/PageHeaderCard'
import { EXPERIENCE_PATH, SKILLS_PATH } from '../../../paths'
import { Field } from '../../Field/Field'
import { PAGE_ACCENT } from '../../JobsPage/jobsTheme'
import { PersonaSwitcher } from '../PersonaSwitcher/PersonaSwitcher'
import { FieldStepper } from './FieldStepper'
import { detailsCompleteness, detailsSteps, type DetailsKey } from './detailsStats'
import { PageTips } from '../../../Shared/PageTips/PageTips'

const emptyForm = {
  headline: '',
  summary: '',
  location: '',
  desiredTitles: '',
  desiredLocations: '',
  minSalary: '',
}

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
export function PersonaDetailsPage() {
  const [personaId, setPersonaId] = useState<string | null>(null)
  const [form, setForm] = useState(emptyForm)
  // What is saved, so the overview does not move while a field is being typed.
  const [saved, setSaved] = useState(emptyForm)
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
          desiredTitles: result.personaDetails?.desiredTitles ?? '',
          desiredLocations: result.personaDetails?.desiredLocations ?? '',
          minSalary: result.personaDetails?.minSalary ? String(result.personaDetails.minSalary) : '',
        }
        setForm(loaded)
        setSaved(loaded)
        setSkillCount(result.skills.length)
        setExperienceCount(result.experience.length)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  useEffect(() => {
    if (personaId) load(personaId)
  }, [personaId])

  function handleSave() {
    if (!personaId) return
    setError('')
    setJustSaved(false)
    setSaving(true)
    updatePersonaDetails(personaId, {
      headline: form.headline,
      summary: form.summary,
      location: form.location,
      desiredTitles: form.desiredTitles,
      desiredLocations: form.desiredLocations,
      minSalary: form.minSalary ? Number(form.minSalary) : undefined,
    })
      .then(() => {
        setSaved(form)
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

  const completeness = detailsCompleteness(saved)

  // The stepper's click: bring that field into view and put the cursor in it.
  function focusField(key: DetailsKey) {
    const el = document.getElementById(`details-${key}`)
    el?.scrollIntoView({ behavior: 'smooth', block: 'center' })
    el?.focus({ preventScroll: true })
  }

  return (
    <div className="career-v-wipe career-a-float career-t-underline max-w-7xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<TargetIcon />}
        title="Summary & Goals"
        description="This persona's positioning and what it is looking for. The AI reads it when matching jobs and writing CVs; nothing here is a secret."
        pill={{ text: `${completeness.percent}% complete` }}
      />
      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <div className="order-1 min-w-0 flex-1 lg:order-2">
          <PersonaSwitcher personaId={personaId} onChange={setPersonaId} />

          <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

          {personaId && (
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

              <h2 className="mb-3 mt-6 text-sm font-semibold text-gray-700">What you want</h2>
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <Field
                  label="Desired titles"
                  id="details-desiredTitles"
                  value={form.desiredTitles}
                  onChange={(v) => {
                    edit({ desiredTitles: v })
                  }}
                />
                <Field
                  label="Desired locations"
                  id="details-desiredLocations"
                  value={form.desiredLocations}
                  onChange={(v) => {
                    edit({ desiredLocations: v })
                  }}
                />
                <Field
                  label="Minimum salary"
                  id="details-minSalary"
                  type="number"
                  value={form.minSalary}
                  onChange={(v) => {
                    edit({ minSalary: v })
                  }}
                />
              </div>

              <div className="mt-5 flex items-center gap-3">
                <button
                  type="button"
                  onClick={handleSave}
                  disabled={saving}
                  className="rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800 disabled:opacity-50"
                >
                  {saving ? 'Saving…' : 'Save details'}
                </button>
                {justSaved && <span className="text-sm text-green-700">Saved</span>}
              </div>
            </section>
          )}
        </div>

        <aside className="order-2 w-full shrink-0 lg:order-1 career-rail-sticky lg:w-72">
          {personaId && (
            <>
              <FieldStepper
                steps={detailsSteps(saved)}
                skillCount={skillCount}
                experienceCount={experienceCount}
                onPick={focusField}
                onOpenSkills={() => {
                  window.__cinqoToolBridge.navigate(SKILLS_PATH)
                }}
                onOpenExperience={() => {
                  window.__cinqoToolBridge.navigate(EXPERIENCE_PATH)
                }}
              />
            </>
          )}
          <div className={personaId ? 'mt-8' : ''}>
            <PageTips page="summary" orientation="vertical" />
          </div>
        </aside>
      </div>
    </div>
  )
}
