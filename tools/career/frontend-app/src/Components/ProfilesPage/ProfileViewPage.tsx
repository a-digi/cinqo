import { useCallback, useEffect, useState } from 'react'
import {
  fetchAllCvDocuments,
  fetchCvAiRuns,
  fetchCvTemplates,
  fetchPersonaDetails,
  fetchPersonas,
  fetchProfiles,
  mediaDownloadUrl,
  type CvDocument,
  type CvTemplate,
  type Persona,
  type PersonaDetailsResult,
  type Profile,
} from '../../api'
import { Comments } from '../../Shared/Comments/Comments'
import { COMMENT_TYPE_PROFILE } from '../../Shared/Comments/commentTypes'
import { UserIcon } from '../../Shared/Icons/icons'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { PROFILES_PATH, personasPathFor, viewPersonaPathFor } from '../../paths'
import { plural } from '../CompaniesPage/companyStats'
import { ProfilePhotosPanel } from '../ProfileImage/ProfilePhotosPanel'
import { useProfilePhotos } from '../ProfileImage/useProfilePhotos'
import { CvCardGrid } from '../CvBuilder/CvCardGrid'
import { chatsByCv } from '../../CvAi/cvAiRules'
import { PAGE_ACCENT, accentFor, initialOf } from '../JobsPage/jobsTheme'
import { profileLabel } from '../ProfileSwitcher/profileLabel'
import { CompletenessMeter } from './CompletenessMeter'
import { CV_LIST_CAP, countsText, cvListMayBeCut, cvsOfProfile, lifetimeText } from './profileViewRules'

const cardStyle = { borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }

function formatDate(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '' : date.toLocaleDateString()
}

// The page behind a profile's "View" button: everything about one profile in one read-only place, its
// details, links and photos, its personas and the CVs that belong to them. Built from the existing
// routes, so it needs no backend of its own. Editing stays on the Profiles page.
export function ProfileViewPage() {
  const profileId = new URLSearchParams(window.location.search).get('id') ?? ''
  const [profile, setProfile] = useState<Profile | null>(null)
  // Reads the profile again (its default photo may have changed after a photo was removed).
  const refreshProfile = useCallback(() => {
    fetchProfiles()
      .then((list) => {
        const found = list.find((p) => p.id === profileId)
        if (found) setProfile(found)
      })
      .catch(() => undefined)
  }, [profileId])
  const { photos, error: photoError, remove: removePhoto } = useProfilePhotos(profileId || null, refreshProfile)
  const [error, setError] = useState('')
  const [loaded, setLoaded] = useState(false)
  const [personas, setPersonas] = useState<Persona[]>([])
  const [personasLoaded, setPersonasLoaded] = useState(false)
  const [details, setDetails] = useState<Record<string, PersonaDetailsResult | null>>({})
  const [cvs, setCvs] = useState<CvDocument[] | null>(null)
  const [totalCvs, setTotalCvs] = useState(0)
  const [templates, setTemplates] = useState<CvTemplate[]>([])
  // The CVs an AI run made, with the chat of the run, to mark them.
  const [aiChats, setAiChats] = useState<ReadonlyMap<string, string>>(new Map())

  useEffect(() => {
    if (!profileId) {
      setError('No profile selected')
      setLoaded(true)
      return
    }
    fetchProfiles()
      .then((list) => {
        const found = list.find((p) => p.id === profileId)
        if (found) setProfile(found)
        else setError('Profile not found')
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setLoaded(true)
      })
    // The rest is best effort: a section that cannot be read is simply left out.
    fetchCvTemplates()
      .then(setTemplates)
      .catch(() => {
        setTemplates([])
      })
    fetchPersonas(profileId)
      .then((list) => {
        setPersonas(list)
        setPersonasLoaded(true)
        void Promise.all(list.map((p) => fetchPersonaDetails(p.id).catch(() => null))).then((all) => {
          setDetails(Object.fromEntries(list.map((p, i) => [p.id, all[i]])))
        })
        void Promise.all(list.map((p) => fetchCvAiRuns(p.id).catch(() => []))).then((all) => {
          setAiChats(chatsByCv(all.flat()))
        })
        fetchAllCvDocuments()
          .then((docs) => {
            setTotalCvs(docs.length)
            setCvs(
              cvsOfProfile(
                docs,
                list.map((p) => p.id),
              ),
            )
          })
          .catch(() => {
            setCvs(null)
          })
      })
      .catch(() => {
        setPersonas([])
        setPersonasLoaded(true)
      })
  }, [profileId])

  function go(e: React.MouseEvent, path: string) {
    e.preventDefault()
    window.__cinqoToolBridge.navigate(path)
  }

  const backLink = (
    <a
      href={PROFILES_PATH}
      onClick={(e) => {
        go(e, PROFILES_PATH)
      }}
      className="mb-3 inline-block text-sm font-medium text-blue-700 hover:underline"
    >
      ← All profiles
    </a>
  )

  if (!profile) {
    return (
      <div className="max-w-3xl p-6 font-sans text-gray-900">
        {backLink}
        <div className="text-sm text-gray-500">{loaded ? error : 'Loading…'}</div>
      </div>
    )
  }

  const name = profileLabel(profile)
  const accent = accentFor(name)
  const defaultPhoto = photos.find((p) => p.isDefault)?.mediaFileId ?? profile.imageMediaFileId

  return (
    <div className="career-v-tilt career-a-float career-t-wave max-w-7xl p-6 font-sans text-gray-900">
      {backLink}
      <PageHeaderCard
        icon={<UserIcon />}
        title={name}
        description={lifetimeText(profile.createdAt, profile.updatedAt, formatDate) || 'No dates recorded.'}
        // The pill links on to this profile's personas (the completeness is in the Overview card below).
        pill={{ text: personasLoaded ? plural(personas.length, 'persona', 'personas') : 'Personas', href: personasPathFor(profile.id) }}
      />

      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <aside className="order-1 w-full shrink-0 space-y-5 lg:w-80">
          <section aria-label="Overview" className="rounded-3xl border p-5 shadow-sm" style={cardStyle}>
            <div className="mb-4 flex items-center gap-3">
              {defaultPhoto ? (
                <img src={mediaDownloadUrl(defaultPhoto)} alt={name} className="h-16 w-16 rounded-full object-cover shadow-sm" />
              ) : (
                <span
                  className="inline-flex h-16 w-16 shrink-0 items-center justify-center rounded-full text-2xl font-bold text-white shadow-sm"
                  style={{ background: accent.to }}
                  aria-hidden="true"
                >
                  {initialOf(name)}
                </span>
              )}
              <div className="min-w-0">
                <p className="truncate text-base font-semibold">{name}</p>
                <p className="text-xs text-gray-500">{personas.length === 1 ? '1 persona' : `${personas.length} personas`}</p>
              </div>
            </div>
            <CompletenessMeter completeness={profile.completeness} />
            <ul className="space-y-1 text-xs">
              {profile.completeness.items.map((item) => (
                <li key={item.key} className={item.done ? 'text-green-700' : 'text-gray-500'}>
                  {item.done ? '✓' : '○'} {item.label}
                </li>
              ))}
            </ul>
          </section>

          <section aria-label="Links" className="rounded-3xl border p-5 shadow-sm" style={cardStyle}>
            <h2 className="mb-2 text-sm font-semibold text-gray-700">Links</h2>
            {profile.externalLinks.length === 0 ? (
              <p className="text-sm text-gray-400">No links yet.</p>
            ) : (
              <ul className="space-y-1.5 text-sm">
                {profile.externalLinks.map((l) => (
                  <li key={l.platform} className="min-w-0">
                    <span className="font-medium text-gray-700">{l.platform}: </span>
                    <a href={l.url} target="_blank" rel="noreferrer" className="break-all text-blue-700 hover:underline">
                      {l.url}
                    </a>
                  </li>
                ))}
              </ul>
            )}
          </section>

          <section aria-label="Photos" className="rounded-3xl border p-5 shadow-sm" style={cardStyle}>
            <h2 className="mb-2 text-sm font-semibold text-gray-700">Photos</h2>
            <ProfilePhotosPanel photos={photos} error={photoError} onRemove={removePhoto} />
          </section>
        </aside>

        <div className="order-2 min-w-0 flex-1 space-y-5">
          <section aria-label="Personas">
            <h2 className="mb-3 text-sm font-semibold text-gray-700">Personas</h2>
            {personas.length === 0 ? (
              <p className="text-sm text-gray-400">This profile has no personas yet.</p>
            ) : (
              <ul className="space-y-3">
                {personas.map((p) => {
                  const d = details[p.id]
                  const facts = [d?.personaDetails?.headline, d?.personaDetails?.desiredTitles, d?.personaDetails?.location].filter(
                    (x): x is string => typeof x === 'string' && x !== '',
                  )
                  return (
                    <li key={p.id} className="rounded-2xl border border-gray-100 bg-white/70 p-3">
                      <div className="flex flex-wrap items-center gap-2">
                        <a
                          href={viewPersonaPathFor(p.id)}
                          onClick={(e) => {
                            go(e, viewPersonaPathFor(p.id))
                          }}
                          className="text-sm font-semibold text-blue-700 hover:underline"
                        >
                          {p.name}
                        </a>
                        {d && <span className="text-xs text-gray-500">{countsText(d.skills.length, d.experience.length)}</span>}
                      </div>
                      {p.description && <p className="mt-0.5 text-xs text-gray-500">{p.description}</p>}
                      {facts.length > 0 && <p className="mt-1 text-xs text-gray-600">{facts.join(' · ')}</p>}
                    </li>
                  )
                })}
              </ul>
            )}
          </section>

          <section aria-label="CVs">
            <h2 className="mb-3 text-sm font-semibold text-gray-700">CVs{cvs !== null ? ` (${cvs.length})` : ''}</h2>
            {cvs === null ? (
              <p className="text-sm text-gray-400">The CVs could not be read.</p>
            ) : cvs.length === 0 ? (
              <p className="text-sm text-gray-400">No CVs yet for this profile&apos;s personas.</p>
            ) : (
              <CvCardGrid docs={cvs} templates={templates} aiChats={aiChats} />
            )}
            {cvs !== null && cvListMayBeCut(totalCvs) && (
              <p className="mt-2 text-xs text-amber-800">
                The CV list shows at most {CV_LIST_CAP} CVs in total, so some of this profile&apos;s CVs may be missing here.
              </p>
            )}
          </section>
        </div>
      </div>

      <Comments entityType={COMMENT_TYPE_PROFILE} entityId={profile.id} />
    </div>
  )
}
