import { useCallback, useEffect, useRef, useState } from 'react'
import {
  fetchPersonas,
  fetchProfiles,
  fetchAllCvDocuments,
  fetchCvTemplates,
  updatePersona,
  deletePersona,
  deletePersonaAiRun,
  type Persona,
  type CvDocument,
  type CvTemplate,
} from '../../../api'
import { LayersIcon, PlusIcon, SparkleIcon } from '../../../Shared/Icons/icons'
import { inputClass } from '../../../Shared/inputClass'
import { PageHeaderCard } from '../../../Shared/PageHeader/PageHeaderCard'
import { NEW_PERSONA_PATH, PERSONAS_PATH, viewPersonaPathFor } from '../../../paths'
import { plural } from '../../CompaniesPage/companyStats'
import { PAGE_ACCENT, accentFor, initialOf } from '../../JobsPage/jobsTheme'
import { PhotoAvatar } from '../../ProfileImage/PhotoAvatar'
import { useProfilePhotos } from '../../ProfileImage/useProfilePhotos'
import { PersonaCvs } from './PersonaCvs'
import { NeedsAttention } from './NeedsAttention'
import { summarizePersonas, undescribedPersonas } from './personaStats'
import { PageTips } from '../../../Shared/PageTips/PageTips'
import { useAiChoice } from '../../../Cinqo/Platform/useAiChoice'
import { profileLabel } from '../../ProfileSwitcher/profileLabel'
import { CreateWithAi } from '../../../PersonaAi/CreateWithAi'
import { PersonaAiHistory } from '../../../PersonaAi/PersonaAiHistory'
import { PersonaSuggestions } from '../../../PersonaAi/PersonaSuggestions'
import { usePersonaAiRuns } from '../../../PersonaAi/usePersonaAiRuns'
import { chatOfRun, hasRunningRun } from '../../../PersonaAi/runDisplay'

// Full persona CRUD — since step 10, every persona belongs to a
// profile (job seeker), so this page now scopes its own list and its
// own create form to the profile in the page's address
// (/tools/career/personas/:profileId), the same "forced to be mapped" enforcement
// one level up from step 8's own personaId requirement. See
// plan/ai/tools/career/step-09-persona-frontend.md and
// plan/ai/tools/career/step-11-job-seeker-profile-frontend.md. Same layout as
// Companies, but mirrored: header card, the personas on the left and the "needs
// attention" panel on the right;
// "New persona" opens its own page.
export function PersonasPage({ profileId }: { profileId: string }) {
  // A chat link to one suggestion (/personas/:profileId?suggestion=…): the suggestion is scrolled to once it is
  // shown. The profile comes from the address, not from a dropdown.
  const [linkedId] = useState(() => new URLSearchParams(window.location.search).get('suggestion'))
  // The profile's name (for the header and the AI chat's title); missing is null, and unknown once loaded.
  const [profileLoaded, setProfileLoaded] = useState(false)
  const [personas, setPersonas] = useState<Persona[]>([])
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editName, setEditName] = useState('')
  const [editDescription, setEditDescription] = useState('')
  const [error, setError] = useState('')
  // Create personas with AI (step 112): the AI in use, this profile's name (for the chat's
  // title), and its runs.
  const ai = useAiChoice()
  const [profileName, setProfileName] = useState('')
  const [profileFound, setProfileFound] = useState(false)
  const aiRuns = usePersonaAiRuns(profileId)
  // The selected profile's default photo, shown on each persona item (a persona has no photo of its own).
  const profilePhotos = useProfilePhotos(profileId)
  const defaultPhotoId = profilePhotos.photos.find((ph) => ph.isDefault)?.mediaFileId
  // The personas the AI just created (step 112): shown highlighted and bigger until dismissed or
  // until another profile is picked, and scrolled to once they are in the list.
  const [newIds, setNewIds] = useState<string[]>([])
  const [scrollToNew, setScrollToNew] = useState(false)

  useEffect(() => {
    setNewIds([])
    setScrollToNew(false)
  }, [profileId])

  useEffect(() => {
    if (!scrollToNew) return
    const first = personas.find((p) => newIds.includes(p.id))
    const el = first ? document.getElementById(`persona-${first.id}`) : null
    if (!el) return
    el.scrollIntoView({ behavior: 'smooth', block: 'center' })
    setScrollToNew(false)
  }, [scrollToNew, personas, newIds])

  // When the AI finishes while the user is here, the suggestions are brought into view.
  const scrolledToLink = useRef(false)
  useEffect(() => {
    if (linkedId === null || scrolledToLink.current) return
    const el = document.getElementById(`suggestion-${linkedId}`)
    if (!el) return
    scrolledToLink.current = true
    el.scrollIntoView({ behavior: 'smooth', block: 'center' })
  }, [linkedId, aiRuns.runs])

  const wasWorking = useRef(false)
  useEffect(() => {
    if (hasRunningRun(aiRuns.runs)) {
      wasWorking.current = true
    } else if (wasWorking.current) {
      wasWorking.current = false
      requestAnimationFrame(() => {
        document.getElementById('persona-suggestions')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
      })
    }
  }, [aiRuns.runs])

  useEffect(() => {
    setProfileLoaded(false)
    fetchProfiles()
      .then((list) => {
        const found = list.find((p) => p.id === profileId)
        setProfileName(found ? profileLabel(found) : '')
        setProfileFound(found !== undefined)
      })
      .catch(() => {
        setProfileName('')
        setProfileFound(false)
      })
      .finally(() => {
        setProfileLoaded(true)
      })
  }, [profileId])

  function load(forProfileId: string) {
    setError('')
    fetchPersonas(forProfileId)
      .then(setPersonas)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  useEffect(() => {
    if (profileId) load(profileId)
  }, [profileId])

  // The CVs of the personas (step 114). When they cannot be read (no access to CVs) the personas
  // simply show no CV section.
  const [cvDocs, setCvDocs] = useState<CvDocument[] | null>(null)
  const [cvTemplates, setCvTemplates] = useState<CvTemplate[]>([])
  const loadCvs = useCallback(() => {
    fetchAllCvDocuments()
      .then(setCvDocs)
      .catch(() => {
        setCvDocs(null)
      })
  }, [])
  useEffect(() => {
    if (!profileId) return
    loadCvs()
    fetchCvTemplates()
      .then(setCvTemplates)
      .catch(() => {
        setCvTemplates([])
      })
  }, [profileId, loadCvs])

  function startEdit(p: Persona) {
    setEditingId(p.id)
    setEditName(p.name)
    setEditDescription(p.description ?? '')
  }

  function cancelEdit() {
    setEditingId(null)
    setEditName('')
    setEditDescription('')
  }

  function saveEdit(id: string) {
    const name = editName.trim()
    if (!name) {
      setError('Name is required')
      return
    }
    setError('')
    updatePersona(id, { name, description: editDescription.trim() })
      .then(() => {
        if (profileId) load(profileId)
        cancelEdit()
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  function handleDelete(id: string) {
    setError('')
    deletePersona(id)
      .then(() => {
        setPersonas((prev) => prev.filter((p) => p.id !== id))
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  const summary = summarizePersonas(personas)
  const undescribed = undescribedPersonas(personas)

  // The panel's "Add one": opens that persona's inline edit and brings it into view.
  function editFromPanel(id: string) {
    const persona = personas.find((p) => p.id === id)
    if (!persona) return
    startEdit(persona)
    requestAnimationFrame(() => {
      document.getElementById(`persona-${id}`)?.scrollIntoView({ behavior: 'smooth', block: 'center' })
    })
  }

  const backLink = (
    <a
      href={PERSONAS_PATH}
      onClick={(e) => {
        e.preventDefault()
        window.__cinqoToolBridge.navigate(PERSONAS_PATH)
      }}
      className="mb-3 inline-block text-sm font-medium text-blue-700 hover:underline"
    >
      ← All profiles
    </a>
  )

  if (!profileFound) {
    return (
      <div className="max-w-3xl p-6 font-sans text-gray-900">
        {backLink}
        <div className="text-sm text-gray-500">{profileLoaded ? 'Profile not found' : 'Loading…'}</div>
      </div>
    )
  }

  return (
    <div className="career-v-flip career-a-sheen career-t-wave max-w-7xl p-6 font-sans text-gray-900">
      {backLink}
      <PageHeaderCard
        icon={<LayersIcon />}
        title={`Personas: ${profileName}`}
        description={
          'One persona per role this profile is pursuing, e.g. "Backend Engineer". Each has its own summary, skills and experience.'
        }
        pill={{ text: plural(summary.personas, 'persona', 'personas') }}
      />
      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <div className="min-w-0 flex-1">
          {profileId && (
            <div className="mb-3 flex items-center justify-start gap-3">
              <button
                type="button"
                onClick={() => {
                  window.__cinqoToolBridge.navigate(`${NEW_PERSONA_PATH}?profileId=${encodeURIComponent(profileId)}`)
                }}
                className="flex shrink-0 items-center gap-1.5 rounded-md bg-gray-800 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-900"
              >
                <PlusIcon />
                New persona
              </button>
              <CreateWithAi
                ai={ai}
                disabledReason={
                  !ai.platform
                    ? 'No AI platform is available. Choose one in Settings.'
                    : personas.length === 0
                      ? 'Add a persona first: the AI builds on what you have filled in.'
                      : hasRunningRun(aiRuns.runs)
                        ? 'An AI run is already working on this profile.'
                        : ''
                }
                onStart={(focus) => aiRuns.start({ id: profileId, name: profileName }, focus, ai)}
              />
              <p className="min-w-0 flex-1 text-xs text-gray-500">
                Deleting a persona permanently deletes its details, every skill and every experience entry, with no further confirmation.
              </p>
            </div>
          )}

          <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

          {newIds.length > 0 && (
            <div
              role="status"
              className="mb-3 flex items-center justify-between gap-3 rounded-2xl border border-purple-200 bg-purple-50 px-4 py-2.5 text-sm text-purple-900"
            >
              <span>
                {newIds.length === 1 ? '1 persona was' : `${newIds.length} personas were`} created with AI.{' '}
                {newIds.length === 1 ? 'It is' : 'They are'} highlighted below.
              </span>
              <button
                type="button"
                onClick={() => {
                  setNewIds([])
                }}
                className="shrink-0 rounded-full border border-purple-200 bg-white px-3 py-1 text-xs font-medium text-purple-800 hover:bg-purple-100"
              >
                Dismiss
              </button>
            </div>
          )}

          {profileId && (
            <PersonaSuggestions
              runs={aiRuns.runs}
              linkedId={linkedId}
              existing={personas.map((p) => ({ id: p.id, name: p.name }))}
              onAccepted={(created) => {
                setNewIds((ids) => [...ids, ...created.map((c) => c.id)])
                setScrollToNew(true)
                load(profileId)
                void aiRuns.reload()
              }}
              onChanged={() => {
                void aiRuns.reload()
              }}
            />
          )}

          {profileId && personas.length === 0 && !error && (
            <div
              className="career-wiggle rounded-3xl border p-8 text-center text-sm text-gray-500"
              style={{ borderColor: PAGE_ACCENT.soft, background: PAGE_ACCENT.soft }}
            >
              This profile has no personas yet. Create the first one with “New persona”.
            </div>
          )}

          {profileId && (
            <div className="career-stagger space-y-3">
              {personas.map((p) => {
                const isNew = newIds.includes(p.id)
                return (
                  <div
                    key={p.id}
                    id={`persona-${p.id}`}
                    data-new={isNew ? 'true' : undefined}
                    className={`rounded-3xl border shadow-sm transition-all hover:shadow-md ${isNew ? 'p-6 ring-2 ring-purple-300' : 'p-4'}`}
                    style={
                      isNew
                        ? {
                            borderColor: '#d8b4fe',
                            background: 'linear-gradient(160deg, #ffffff 25%, #f3e8ff 100%)',
                            // No entrance animation: the list's stagger delays a card by its position
                            // (over a second for the ninth), and a new persona must be there at once.
                            animation: 'none',
                          }
                        : { borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }
                    }
                  >
                    {editingId === p.id ? (
                      <div>
                        <input
                          // Deliberate: entering edit mode should focus the input
                          // immediately, the same convention most inline-rename UIs use.
                          // eslint-disable-next-line jsx-a11y/no-autofocus
                          autoFocus
                          value={editName}
                          onChange={(e) => {
                            setEditName(e.target.value)
                          }}
                          placeholder="Name"
                          className={`mb-2 ${inputClass}`}
                        />
                        <input
                          value={editDescription}
                          onChange={(e) => {
                            setEditDescription(e.target.value)
                          }}
                          placeholder="Description"
                          className={`mb-2 ${inputClass}`}
                        />
                        <div className="flex gap-2">
                          <button
                            type="button"
                            onClick={() => {
                              saveEdit(p.id)
                            }}
                            className="rounded-md bg-gray-900 px-3 py-1 text-xs font-medium text-white hover:bg-gray-800"
                          >
                            Save
                          </button>
                          <button
                            type="button"
                            onClick={cancelEdit}
                            className="rounded-md border border-gray-200 px-3 py-1 text-xs text-gray-700 hover:bg-gray-50"
                          >
                            Cancel
                          </button>
                        </div>
                      </div>
                    ) : (
                      <div className="flex items-start gap-3">
                        {profileId && defaultPhotoId ? (
                          <PhotoAvatar
                            profileId={profileId}
                            mediaFileId={defaultPhotoId}
                            sizeClass={isNew ? 'h-14 w-14' : 'h-12 w-12'}
                            onChanged={profilePhotos.reload}
                            onError={setError}
                          />
                        ) : (
                          <span
                            className={`inline-flex shrink-0 items-center justify-center rounded-full font-bold text-white shadow-sm ${isNew ? 'h-12 w-12 text-lg' : 'h-10 w-10 text-base'}`}
                            style={{ background: accentFor(p.name).to }}
                            aria-hidden="true"
                          >
                            {initialOf(p.name)}
                          </span>
                        )}
                        <div className="min-w-0 flex-1">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className={isNew ? 'text-base font-semibold' : 'text-sm font-semibold'}>{p.name}</span>
                            {isNew && (
                              <span className="rounded-full bg-purple-600 px-2 py-0.5 text-[11px] font-semibold text-white">New</span>
                            )}
                            {p.aiRunId !== undefined && <CreatedWithAiChip chatId={chatOfRun(aiRuns.runs, p.aiRunId)} />}
                          </div>
                          {p.description && <div className="mt-0.5 text-xs text-gray-500">{p.description}</div>}
                          <div className="mt-1 text-xs text-gray-400">Created {formatDate(p.createdAt)}</div>
                          {cvDocs !== null && (
                            <PersonaCvs
                              personaId={p.id}
                              personaName={p.name}
                              docs={cvDocs}
                              templates={cvTemplates}
                              ai={ai}
                              onCvsChanged={loadCvs}
                            />
                          )}
                        </div>
                        <div className="flex shrink-0 gap-2">
                          <button
                            type="button"
                            onClick={() => {
                              window.__cinqoToolBridge.navigate(viewPersonaPathFor(p.id))
                            }}
                            className="rounded-full border border-gray-200 px-3 py-1 text-xs text-gray-700 hover:bg-gray-50"
                          >
                            View
                          </button>
                          <button
                            type="button"
                            onClick={() => {
                              startEdit(p)
                            }}
                            className="rounded-full border border-blue-200 px-3 py-1 text-xs text-blue-700 hover:bg-blue-50"
                          >
                            Edit
                          </button>
                          <button
                            type="button"
                            onClick={() => {
                              handleDelete(p.id)
                            }}
                            className="rounded-full border border-red-200 px-3 py-1 text-xs text-red-700 hover:bg-red-50"
                          >
                            Delete
                          </button>
                        </div>
                      </div>
                    )}
                  </div>
                )
              })}
            </div>
          )}
        </div>

        <aside className="w-full shrink-0 lg:sticky lg:top-6 lg:w-72">
          {profileId && (
            <>
              <PersonaAiHistory
                runs={aiRuns.runs}
                onReview={() => {
                  document.getElementById('persona-suggestions')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
                }}
                onRemove={(run) =>
                  deletePersonaAiRun(run.id).then(() => {
                    // The personas it created stay, without the link to the run.
                    load(profileId)
                    return aiRuns.reload()
                  })
                }
              />
              <NeedsAttention total={summary.personas} undescribed={undescribed} onEdit={editFromPanel} />
            </>
          )}
          <div className={profileId ? 'mt-8' : ''}>
            <PageTips page="personas" orientation="vertical" />
          </div>
        </aside>
      </div>
    </div>
  )
}

// The created time as a local date, or as given when it is not one.
function formatDate(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleDateString()
}

// "Created with AI" on a persona that came from a run; with its chat still in the history it opens
// that chat. Personas are the only place this shows (step 112).
function CreatedWithAiChip({ chatId }: { chatId: string | null }) {
  const look = 'inline-flex items-center gap-1 rounded-full bg-purple-50 px-2 py-0.5 text-[11px] font-medium text-purple-700'
  if (chatId === null) {
    return (
      <span className={look}>
        <SparkleIcon />
        Created with AI
      </span>
    )
  }
  return (
    <button
      type="button"
      title="Open the chat that proposed this persona"
      onClick={() => {
        window.__cinqoToolBridge.openConversation(chatId)
      }}
      className={`${look} hover:bg-purple-100`}
    >
      <SparkleIcon />
      Created with AI
    </button>
  )
}
