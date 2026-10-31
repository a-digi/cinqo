import { useEffect, useState } from 'react'
import {
  fetchAllCvDocuments,
  fetchCvAiRuns,
  fetchCvTemplates,
  fetchPersonaAiRuns,
  fetchPersonaDetails,
  fetchPersonas,
  fetchProfiles,
  type CvDocument,
  type CvTemplate,
  type Persona,
  type PersonaDetailsResult,
  type Profile,
} from '../../../api'
import { Comments } from '../../../Shared/Comments/Comments'
import { COMMENT_TYPE_PERSONA } from '../../../Shared/Comments/commentTypes'
import { LayersIcon, SparkleIcon } from '../../../Shared/Icons/icons'
import { PageHeaderCard } from '../../../Shared/PageHeader/PageHeaderCard'
import { PERSONAS_PATH, PERSONA_DETAILS_PATH, VIEW_PROFILE_PATH } from '../../../paths'
import { chatOfRun } from '../../../PersonaAi/runDisplay'
import { ProfilePhotosPanel } from '../../ProfileImage/ProfilePhotosPanel'
import { useProfilePhotos } from '../../ProfileImage/useProfilePhotos'
import { CvCardGrid } from '../../CvBuilder/CvCardGrid'
import { chatsByCv } from '../../../CvAi/cvAiRules'
import { PAGE_ACCENT } from '../../JobsPage/jobsTheme'
import { cvsOfProfile, lifetimeText } from '../../ProfilesPage/profileViewRules'
import { setProfileIdInLocalStorage } from '../../ProfileSwitcher/repositoryLocalStorage'
import { profileLabel } from '../../ProfileSwitcher/profileLabel'
import { setPersonaIdInLocalStorage } from '../PersonaSwitcher/repositoryLocalStorage'
import { detailRows, nothingFilledIn, roleView } from './personaViewRules'

const cardStyle = { borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }

function formatDate(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '' : date.toLocaleDateString()
}

// The page behind a persona's "View" button: everything about one persona in one read-only place, its
// summary and goals, skills and experience, and the CVs built from it or attached to it. Built from the
// existing routes, so it needs no backend of its own. Editing stays on the Personas page and the Persona
// Details page.
export function PersonaViewPage() {
  const personaId = new URLSearchParams(window.location.search).get('id') ?? ''
  const [persona, setPersona] = useState<Persona | null>(null)
  const [profile, setProfile] = useState<Profile | null>(null)
  const [error, setError] = useState('')
  const [loaded, setLoaded] = useState(false)
  const [details, setDetails] = useState<PersonaDetailsResult | null>(null)
  const [detailsFailed, setDetailsFailed] = useState(false)
  const [cvs, setCvs] = useState<CvDocument[] | null>(null)
  const [templates, setTemplates] = useState<CvTemplate[]>([])
  // The CVs an AI run made (their ids), to mark them; and the chat of the run that made this persona.
  const [aiChats, setAiChats] = useState<ReadonlyMap<string, string>>(new Map())
  const [personaChatId, setPersonaChatId] = useState<string | null>(null)
  // The photos are the profile's (a persona has none of its own).
  const { photos, error: photoError, remove: removePhoto } = useProfilePhotos(persona?.profileId ?? null)

  useEffect(() => {
    if (!personaId) {
      setError('No persona selected')
      setLoaded(true)
      return
    }
    fetchPersonas()
      .then((list) => {
        const found = list.find((p) => p.id === personaId)
        if (!found) {
          setError('Persona not found')
          return
        }
        setPersona(found)
        // The rest is best effort: a section that cannot be read is simply left out.
        fetchProfiles()
          .then((profiles) => {
            setProfile(profiles.find((p) => p.id === found.profileId) ?? null)
          })
          .catch(() => {
            setProfile(null)
          })
        if (found.aiRunId !== undefined) {
          fetchPersonaAiRuns(found.profileId)
            .then((runs) => {
              setPersonaChatId(chatOfRun(runs, found.aiRunId))
            })
            .catch(() => {
              setPersonaChatId(null)
            })
        }
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setLoaded(true)
      })
    fetchPersonaDetails(personaId)
      .then(setDetails)
      .catch(() => {
        setDetailsFailed(true)
      })
    fetchCvTemplates()
      .then(setTemplates)
      .catch(() => {
        setTemplates([])
      })
    fetchAllCvDocuments()
      .then((docs) => {
        setCvs(cvsOfProfile(docs, [personaId]))
      })
      .catch(() => {
        setCvs(null)
      })
    fetchCvAiRuns(personaId)
      .then((runs) => {
        setAiChats(chatsByCv(runs))
      })
      .catch(() => {
        setAiChats(new Map())
      })
  }, [personaId])

  function go(e: React.MouseEvent, path: string, beforeGo?: () => void) {
    e.preventDefault()
    beforeGo?.()
    window.__cinqoToolBridge.navigate(path)
  }

  const backLink = (
    <a
      href={PERSONAS_PATH}
      onClick={(e) => {
        go(e, PERSONAS_PATH, () => {
          if (persona) setProfileIdInLocalStorage(persona.profileId)
        })
      }}
      className="mb-3 inline-block text-sm font-medium text-blue-700 hover:underline"
    >
      ← All personas
    </a>
  )

  if (!persona) {
    return (
      <div className="max-w-3xl p-6 font-sans text-gray-900">
        {backLink}
        <div className="text-sm text-gray-500">{loaded ? error : 'Loading…'}</div>
      </div>
    )
  }

  const rows = detailRows(details?.personaDetails ?? null)
  const summary = details?.personaDetails?.summary.trim() ?? ''
  const skills = details?.skills ?? []
  const experience = details?.experience ?? []
  const profilePath = `${VIEW_PROFILE_PATH}?id=${encodeURIComponent(persona.profileId)}`

  return (
    <div className="career-v-tilt career-a-float career-t-wave max-w-7xl p-6 font-sans text-gray-900">
      {backLink}
      <PageHeaderCard
        icon={<LayersIcon />}
        title={persona.name}
        description={persona.description ?? 'No description yet.'}
        pill={profile ? { text: profileLabel(profile) } : undefined}
      />
      <p className="mb-5 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-gray-500">
        <span>{lifetimeText(persona.createdAt, persona.updatedAt, formatDate)}</span>
        {profile && (
          <a
            href={profilePath}
            onClick={(e) => {
              go(e, profilePath)
            }}
            className="font-medium text-blue-700 hover:underline"
          >
            Profile: {profileLabel(profile)}
          </a>
        )}
        {persona.aiRunId !== undefined &&
          (personaChatId !== null ? (
            <button
              type="button"
              title="Open the chat that proposed this persona"
              onClick={() => {
                window.__cinqoToolBridge.openConversation(personaChatId)
              }}
              className="inline-flex items-center gap-1 rounded-full bg-purple-50 px-2 py-0.5 text-[11px] font-medium text-purple-700 hover:bg-purple-100"
            >
              <SparkleIcon />
              Created with AI
            </button>
          ) : (
            <span className="inline-flex items-center gap-1 rounded-full bg-purple-50 px-2 py-0.5 text-[11px] font-medium text-purple-700">
              <SparkleIcon />
              Created with AI
            </span>
          ))}
      </p>

      <div className="space-y-5">
        <section aria-label="Summary and goals" className="rounded-3xl border p-5 shadow-sm" style={cardStyle}>
          <div className="mb-3 flex flex-wrap items-center gap-2">
            <h2 className="text-sm font-semibold text-gray-700">Summary &amp; goals</h2>
            <button
              type="button"
              onClick={() => {
                setPersonaIdInLocalStorage(persona.id)
                window.__cinqoToolBridge.navigate(PERSONA_DETAILS_PATH)
              }}
              className="ml-auto text-xs text-blue-700 hover:underline"
            >
              Edit details
            </button>
          </div>
          {detailsFailed ? (
            <p className="text-sm text-gray-400">The details could not be read.</p>
          ) : nothingFilledIn(details?.personaDetails ?? null) ? (
            <p className="text-sm text-gray-400">Nothing is filled in yet. Add the headline, summary and goals with “Edit details”.</p>
          ) : (
            <>
              <dl className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                {rows.map((r) => (
                  <div key={r.label} className="min-w-0">
                    <dt className="text-[11px] font-medium uppercase tracking-wide text-gray-400">{r.label}</dt>
                    <dd className="mt-0.5 break-words text-sm text-gray-800">
                      {r.value === '' ? <span className="text-gray-400">Not set</span> : r.value}
                    </dd>
                  </div>
                ))}
              </dl>
              <p className="mt-3 whitespace-pre-wrap break-words text-sm text-gray-800">
                {summary === '' ? <span className="text-gray-400">No summary.</span> : summary}
              </p>
            </>
          )}
        </section>

        <div className="grid gap-5 lg:grid-cols-2">
          <section aria-label="Skills" className="rounded-3xl border p-5 shadow-sm" style={cardStyle}>
            <h2 className="mb-3 text-sm font-semibold text-gray-700">Skills ({skills.length})</h2>
            {skills.length === 0 ? (
              <p className="text-sm text-gray-400">No skills yet.</p>
            ) : (
              <ul className="flex flex-wrap gap-1.5">
                {skills.map((s) => (
                  <li
                    key={s}
                    className="rounded-full px-2.5 py-1 text-xs"
                    style={{ background: PAGE_ACCENT.soft, color: PAGE_ACCENT.text }}
                  >
                    {s}
                  </li>
                ))}
              </ul>
            )}
          </section>

          <section aria-label="Experience" className="rounded-3xl border p-5 shadow-sm" style={cardStyle}>
            <h2 className="mb-3 text-sm font-semibold text-gray-700">Experience ({experience.length})</h2>
            {experience.length === 0 ? (
              <p className="text-sm text-gray-400">No experience entries yet.</p>
            ) : (
              <ul className="space-y-2.5">
                {experience.map(roleView).map((r) => (
                  <li key={r.id} className="rounded-2xl border border-gray-100 bg-white/70 p-3">
                    <p className="text-sm font-medium text-gray-900">
                      {r.title} <span className="font-normal text-gray-500">at {r.company}</span>
                    </p>
                    {r.period !== '' && <p className="text-xs text-gray-400">{r.period}</p>}
                    {r.description !== '' && <p className="mt-1 whitespace-pre-wrap break-words text-xs text-gray-600">{r.description}</p>}
                  </li>
                ))}
              </ul>
            )}
          </section>
        </div>

        <section aria-label="Photos" className="rounded-3xl border p-5 shadow-sm" style={cardStyle}>
          <h2 className="mb-3 text-sm font-semibold text-gray-700">Photos ({photos.length})</h2>
          <ProfilePhotosPanel photos={photos} error={photoError} onRemove={removePhoto} />
        </section>

        <section aria-label="CVs">
          <h2 className="mb-3 text-sm font-semibold text-gray-700">CVs{cvs !== null ? ` (${cvs.length})` : ''}</h2>
          {cvs === null ? (
            <p className="text-sm text-gray-400">The CVs could not be read.</p>
          ) : cvs.length === 0 ? (
            <p className="text-sm text-gray-400">No CVs yet for this persona.</p>
          ) : (
            <CvCardGrid docs={cvs} templates={templates} aiChats={aiChats} />
          )}
        </section>
      </div>

      <Comments entityType={COMMENT_TYPE_PERSONA} entityId={persona.id} />
    </div>
  )
}
