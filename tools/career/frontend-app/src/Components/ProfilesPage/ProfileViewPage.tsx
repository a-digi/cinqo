import { useEffect, useState } from 'react'
import {
  fetchAllCvDocuments,
  fetchCvAiRuns,
  fetchCvTemplates,
  fetchPersonaDetails,
  fetchPersonas,
  fetchProfilePhotos,
  fetchProfiles,
  mediaDownloadUrl,
  type CvDocument,
  type CvTemplate,
  type Persona,
  type PersonaDetailsResult,
  type Profile,
  type ProfilePhoto,
} from '../../api'
import { Comments } from '../../Shared/Comments/Comments'
import { COMMENT_TYPE_PROFILE } from '../../Shared/Comments/commentTypes'
import { UserIcon } from '../../Shared/Icons/icons'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { PROFILES_PATH, VIEW_PERSONA_PATH } from '../../paths'
import { CvList } from '../CvBuilder/CvList'
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
  const [error, setError] = useState('')
  const [loaded, setLoaded] = useState(false)
  const [photos, setPhotos] = useState<ProfilePhoto[]>([])
  const [personas, setPersonas] = useState<Persona[]>([])
  const [details, setDetails] = useState<Record<string, PersonaDetailsResult | null>>({})
  const [cvs, setCvs] = useState<CvDocument[] | null>(null)
  const [totalCvs, setTotalCvs] = useState(0)
  const [templates, setTemplates] = useState<CvTemplate[]>([])
  // The CVs an AI run made (their ids), to mark them.
  const [aiCvIds, setAiCvIds] = useState<ReadonlySet<string>>(new Set())

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
    fetchProfilePhotos(profileId)
      .then(setPhotos)
      .catch(() => {
        setPhotos([])
      })
    fetchCvTemplates()
      .then(setTemplates)
      .catch(() => {
        setTemplates([])
      })
    fetchPersonas(profileId)
      .then((list) => {
        setPersonas(list)
        void Promise.all(list.map((p) => fetchPersonaDetails(p.id).catch(() => null))).then((all) => {
          setDetails(Object.fromEntries(list.map((p, i) => [p.id, all[i]])))
        })
        void Promise.all(list.map((p) => fetchCvAiRuns(p.id).catch(() => []))).then((all) => {
          setAiCvIds(new Set(all.flat().flatMap((r) => (r.cvDocumentId === undefined ? [] : [r.cvDocumentId]))))
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
        pill={{ text: `${profile.completeness.score}% complete` }}
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
            {photos.length === 0 ? (
              <p className="text-sm text-gray-400">No photos yet.</p>
            ) : (
              <ul className="flex flex-wrap gap-2">
                {photos.map((p) => (
                  <li key={p.mediaFileId} className="relative">
                    <img
                      src={mediaDownloadUrl(p.mediaFileId)}
                      alt={p.isDefault ? 'Default' : ''}
                      className={`h-16 w-16 rounded-xl object-cover ${p.isDefault ? 'ring-2 ring-purple-400' : ''}`}
                    />
                    {p.isDefault && (
                      <span className="absolute -bottom-1 left-1/2 -translate-x-1/2 rounded-full bg-purple-600 px-1.5 text-[9px] font-semibold text-white">
                        Default
                      </span>
                    )}
                  </li>
                ))}
              </ul>
            )}
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
                          href={`${VIEW_PERSONA_PATH}?id=${encodeURIComponent(p.id)}`}
                          onClick={(e) => {
                            go(e, `${VIEW_PERSONA_PATH}?id=${encodeURIComponent(p.id)}`)
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

          <section aria-label="CVs" className="rounded-3xl border p-5 shadow-sm" style={cardStyle}>
            <h2 className="mb-3 text-sm font-semibold text-gray-700">CVs{cvs !== null ? ` (${cvs.length})` : ''}</h2>
            {cvs === null ? (
              <p className="text-sm text-gray-400">The CVs could not be read.</p>
            ) : cvs.length === 0 ? (
              <p className="text-sm text-gray-400">No CVs yet for this profile&apos;s personas.</p>
            ) : (
              <CvList docs={cvs} templates={templates} aiCvIds={aiCvIds} />
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
