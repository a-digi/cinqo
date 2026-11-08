import { useEffect, useState } from 'react'
import { addEducation, fetchEducation, fetchProfiles, updateEducation } from '../../api'
import { EducationIcon, PlusIcon } from '../../Shared/Icons/icons'
import { inputClass } from '../../Shared/inputClass'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { EDUCATION_PATH } from '../../paths'
import { Field } from '../Field/Field'
import { PAGE_ACCENT } from '../JobsPage/jobsTheme'
import { buildProfileSearch } from '../ProfileLibrary/libraryRules'
import { profileLabel } from '../ProfileSwitcher/profileLabel'
import { formProblem } from './educationRules'

const emptyEntry = { institution: '', degree: '', fieldOfStudy: '', startDate: '', endDate: '', description: '' }

// The page behind "Add education" and each entry's "Edit": ?profileId= says whose education, ?id= which entry to edit (none = add). The
// profile cannot be changed here. Menu-less, like the other add/edit pages.
export function EducationEntryPage() {
  const params = new URLSearchParams(window.location.search)
  const profileId = params.get('profileId') ?? ''
  const entryId = params.get('id') ?? ''
  const [profileName, setProfileName] = useState('')
  const [form, setForm] = useState(emptyEntry)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const [isLoaded, setIsLoaded] = useState(false)
  const [isSaving, setIsSaving] = useState(false)

  useEffect(() => {
    fetchProfiles()
      .then(async (profiles) => {
        const profile = profiles.find((p) => p.id === profileId)
        if (!profile) {
          setMessage('That profile was not found.')
          return
        }
        setProfileName(profileLabel(profile))
        if (entryId) {
          const entry = (await fetchEducation(profile.id)).find((e) => e.id === entryId)
          if (!entry) {
            setMessage('That education entry was not found.')
            return
          }
          setForm({
            institution: entry.institution,
            degree: entry.degree,
            fieldOfStudy: entry.fieldOfStudy,
            startDate: entry.startDate,
            endDate: entry.endDate,
            description: entry.description,
          })
        }
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setIsLoaded(true)
      })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const backPath = `${EDUCATION_PATH}${buildProfileSearch(profileId)}`
  function backToList() {
    window.__cinqoToolBridge.navigate(backPath)
  }

  function handleSave() {
    const problem = formProblem(form)
    if (problem !== '') {
      setError(problem)
      return
    }
    setError('')
    setIsSaving(true)
    const saved = entryId ? updateEducation(entryId, form) : addEducation(profileId, form)
    saved.then(backToList).catch((err: unknown) => {
      setIsSaving(false)
      setError(err instanceof Error ? err.message : String(err))
    })
  }

  return (
    <div className="career-v-zoom career-a-float career-t-type max-w-3xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<EducationIcon />}
        title={entryId ? 'Edit education' : 'Add education'}
        description={profileName ? `A school, degree or course of ${profileName}.` : 'A school, degree or course of a profile.'}
      />
      {!isLoaded ? (
        <p className="text-sm text-gray-500">Loading…</p>
      ) : message ? (
        <div
          className="rounded-3xl border p-6 text-sm text-gray-700"
          style={{ borderColor: PAGE_ACCENT.soft, background: PAGE_ACCENT.soft }}
        >
          {message}{' '}
          <a
            href={backPath}
            onClick={(e) => {
              e.preventDefault()
              backToList()
            }}
            className="font-medium underline"
          >
            Back to education
          </a>
        </div>
      ) : (
        <form
          onSubmit={(e) => {
            e.preventDefault()
            handleSave()
          }}
          className="career-enter career-delay-1 rounded-3xl border p-5 shadow-sm"
          style={{ borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }}
        >
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Field
              label="Institution"
              value={form.institution}
              onChange={(v) => {
                setForm({ ...form, institution: v })
              }}
            />
            <Field
              label="Degree"
              value={form.degree}
              onChange={(v) => {
                setForm({ ...form, degree: v })
              }}
            />
            <Field
              label="Field of study"
              value={form.fieldOfStudy}
              onChange={(v) => {
                setForm({ ...form, fieldOfStudy: v })
              }}
            />
            <span />
            <Field
              label="Start date (YYYY, YYYY-MM)"
              value={form.startDate}
              onChange={(v) => {
                setForm({ ...form, startDate: v })
              }}
            />
            <Field
              label="End date (empty = still going on)"
              value={form.endDate}
              onChange={(v) => {
                setForm({ ...form, endDate: v })
              }}
            />
          </div>
          <label className="mt-3 block text-xs font-medium text-gray-500">
            Description
            <textarea
              value={form.description}
              onChange={(e) => {
                setForm({ ...form, description: e.target.value })
              }}
              rows={4}
              className={`mt-1 ${inputClass}`}
            />
          </label>
          <div className="min-h-[1.2em] pt-3 text-sm text-red-700">{error}</div>
          <div className="mt-3 flex gap-2">
            <button
              type="submit"
              disabled={isSaving}
              className="flex items-center gap-1 rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800 disabled:opacity-50"
            >
              {!entryId && <PlusIcon />}
              {entryId ? 'Save changes' : 'Add education'}
            </button>
            <button
              type="button"
              onClick={backToList}
              className="rounded-md border border-gray-200 px-3.5 py-2 text-sm text-gray-700 hover:bg-gray-50"
            >
              Cancel
            </button>
          </div>
        </form>
      )}
    </div>
  )
}
