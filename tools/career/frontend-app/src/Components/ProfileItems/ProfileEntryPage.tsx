import { useEffect, useState, type ReactNode } from 'react'
import { fetchProfiles } from '../../api'
import { PlusIcon } from '../../Shared/Icons/icons'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { PAGE_ACCENT } from '../JobsPage/jobsTheme'
import { buildProfileSearch } from '../ProfileLibrary/libraryRules'
import { profileLabel } from '../ProfileSwitcher/profileLabel'

// The add/edit page of a profile's own items (languages, courses, hobbies; plan 231), the same layout as the Education entry page:
// ?profileId= says whose, ?id= which one to edit (none = add). The profile cannot be changed here. Menu-less, like the other add/edit pages.
// Each page says what the form holds (renderFields), how a saved one is read back (load), what stops a save (problem) and how it is saved.
export function ProfileEntryPage<F>({
  listPath,
  icon,
  noun,
  emptyForm,
  load,
  problem,
  save,
  renderFields,
  backLabel,
}: {
  listPath: string
  icon: ReactNode
  // What one item is called, lower case: "language", "course", "hobby" (for the titles and messages).
  noun: string
  emptyForm: F
  // The form of the saved item, or null when the profile has no item with that id.
  load: (profileId: string, entryId: string) => Promise<F | null>
  problem: (form: F) => string
  save: (profileId: string, entryId: string, form: F) => Promise<unknown>
  renderFields: (form: F, setForm: (next: F) => void) => ReactNode
  backLabel: string
}) {
  const params = new URLSearchParams(window.location.search)
  const profileId = params.get('profileId') ?? ''
  const entryId = params.get('id') ?? ''
  const [profileName, setProfileName] = useState('')
  const [form, setForm] = useState<F>(emptyForm)
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
          const existing = await load(profile.id, entryId)
          if (existing === null) {
            setMessage(`That ${noun} was not found.`)
            return
          }
          setForm(existing)
        }
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setIsLoaded(true)
      })
    // the page's functions are module-level and the address does not change while it is open
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const backPath = `${listPath}${buildProfileSearch(profileId)}`
  function backToList() {
    window.__cinqoToolBridge.navigate(backPath)
  }

  function handleSave() {
    const stop = problem(form)
    if (stop !== '') {
      setError(stop)
      return
    }
    setError('')
    setIsSaving(true)
    save(profileId, entryId, form)
      .then(backToList)
      .catch((err: unknown) => {
        setIsSaving(false)
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  return (
    <div className="career-v-zoom career-a-float career-t-type max-w-3xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={icon}
        title={entryId ? `Edit ${noun}` : `Add ${noun}`}
        description={profileName ? `A ${noun} of ${profileName}.` : `A ${noun} of a profile.`}
      />
      {!isLoaded ? (
        <p className="text-sm text-gray-500">Loading…</p>
      ) : message ? (
        <div className="rounded-3xl border p-6 text-sm text-gray-700" style={{ borderColor: PAGE_ACCENT.soft, background: PAGE_ACCENT.soft }}>
          {message}{' '}
          <a
            href={backPath}
            onClick={(e) => {
              e.preventDefault()
              backToList()
            }}
            className="font-medium underline"
          >
            {backLabel}
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
          {renderFields(form, setForm)}
          <div className="min-h-[1.2em] pt-3 text-sm text-red-700">{error}</div>
          <div className="mt-3 flex gap-2">
            <button
              type="submit"
              disabled={isSaving}
              className="flex items-center gap-1 rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800 disabled:opacity-50"
            >
              {!entryId && <PlusIcon />}
              {entryId ? 'Save changes' : `Add ${noun}`}
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
