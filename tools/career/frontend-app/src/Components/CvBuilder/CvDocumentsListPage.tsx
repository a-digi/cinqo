import { useEffect, useState } from 'react'
import { fetchAllCvDocuments, fetchPersonas, fetchProfiles, type CvDocument, type Persona, type Profile } from '../../api'
import { PDFIcon } from '../../Shared/Icons/icons'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { PageTips } from '../../Shared/PageTips/PageTips'
import { CV_DOCUMENTS_PATH, PERSONAS_PATH, cvDocumentsPathFor } from '../../paths'
import { plural } from '../CompaniesPage/companyStats'
import { PAGE_ACCENT } from '../JobsPage/jobsTheme'
import { PersonaList } from '../Persona/PersonaList'
import { previewItems } from '../Persona/personaListRules'
import { cvListMayBeCut } from '../ProfilesPage/profileViewRules'
import { cvCountOfProfile, cvCountText, cvsOfPersona } from './cvPersonaRules'
import { AllCvsList } from './AllCvsList'
import { useCvDeletion } from './useCvDeletion'

// How many of a persona's CV titles the list shows before "+N more".
const PREVIEW_CVS = 3

// The CVs page: the personas as a list (grouped by profile), each with how many CVs it has (built from it or
// attached to it) and the first few titles; a click opens that persona's own CV page
// (/tools/career/cv-documents/:personaId), where the CVs are shown as cards and created. Under the personas, "All CVs"
// lists every CV with a Delete (step 3 of plan/ai/tools/career/cv-delete-from-list.md); nothing else is edited here,
// and there is no dropdown. See plan/ai/tools/career/step-122-cv-documents-per-persona-pages.md.
export function CvDocumentsListPage() {
  const [personas, setPersonas] = useState<Persona[]>([])
  const [profiles, setProfiles] = useState<Profile[]>([])
  // Every CV (with its personas); null when they could not be read, so no count is shown.
  const [docs, setDocs] = useState<CvDocument[] | null>(null)
  // True when reading the CVs failed: then no count is shown (a placeholder is shown only while they load).
  const [docsFailed, setDocsFailed] = useState(false)
  const [error, setError] = useState('')
  // What went wrong with a delete (a failed file delete says the CV was kept); '' when nothing did.
  const [deleteError, setDeleteError] = useState('')
  const [loaded, setLoaded] = useState(false)
  // The delete flow is shared with the persona's own CV page: its confirmations, then the CV and its PDF are gone and the list (and
  // every persona's count) follows.
  const deletion = useCvDeletion({
    onDeleted: (doc) => {
      setDocs((prev) => (prev === null ? prev : prev.filter((d) => d.id !== doc.id)))
    },
    onError: setDeleteError,
  })

  useEffect(() => {
    Promise.all([fetchPersonas(), fetchProfiles()])
      .then(([personaList, profileList]) => {
        setPersonas(personaList)
        setProfiles(profileList)
        fetchAllCvDocuments()
          .then(setDocs)
          .catch(() => {
            setDocs(null)
            setDocsFailed(true)
          })
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setLoaded(true)
      })
  }, [])

  return (
    <div className="career-v-pop career-a-float career-t-type max-w-7xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<PDFIcon className="h-6 w-6" />}
        title="CV Documents"
        help={CV_DOCUMENTS_PATH}
        description="Pick a target role to see its CVs and create new ones from its own data, using one of a few fixed designs."
        pill={loaded && error === '' ? { text: plural(personas.length, 'target role', 'target roles') } : undefined}
      />
      {/* The tips sit in a column of their own beside the list (under it on a narrow window). */}
      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <div className="order-1 min-w-0 flex-1">
          {error !== '' && <p className="mb-3 text-sm text-red-700">{error}</p>}
          {loaded && error === '' && personas.length === 0 && (
            <p className="rounded-3xl border border-gray-100 bg-gray-50 p-8 text-center text-sm text-gray-500">
              There are no target roles yet.{' '}
              <a
                href={PERSONAS_PATH}
                onClick={(e) => {
                  e.preventDefault()
                  window.__cinqoToolBridge.navigate(PERSONAS_PATH)
                }}
                className="font-medium text-blue-700 hover:underline"
              >
                Create one on the Target roles page
              </a>
              , then create its CVs here.
            </p>
          )}
          {personas.length > 0 && (
            <>
              {docs !== null && (
                <p className="mb-4 text-xs text-gray-500">
                  {plural(docs.length, 'CV', 'CVs')} in total.
                  {cvListMayBeCut(docs.length) && ' The list shows at most 200 CVs, so some may be missing.'}
                </p>
              )}
              {deleteError !== '' && (
                <p role="alert" className="mb-3 text-sm text-red-700">
                  {deleteError}
                </p>
              )}
              <PersonaList
                personas={personas}
                profiles={profiles}
                hrefFor={(p) => cvDocumentsPathFor(p.id)}
                renderGroupExtra={(group) => {
                  if (docsFailed) return null
                  if (docs === null) return <CountPill text="" />
                  return (
                    <CountPill
                      text={cvCountText(
                        cvCountOfProfile(
                          docs,
                          group.personas.map((p) => p.id),
                        ),
                      )}
                    />
                  )
                }}
                renderExtra={(p) => {
                  if (docsFailed) return null
                  if (docs === null)
                    return (
                      <span className="mt-1.5 block">
                        <CountPill text="" />
                      </span>
                    )
                  const mine = cvsOfPersona(docs, p.id)
                  const { shown, more } = previewItems(
                    mine.map((m) => m.doc.title),
                    PREVIEW_CVS,
                  )
                  return (
                    <span className="mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-gray-500">
                      <CountPill text={cvCountText(mine.length)} />
                      {shown.length > 0 && <span>{shown.join(' · ')}</span>}
                      {more > 0 && <span>+{more} more</span>}
                    </span>
                  )
                }}
              />
              {docs !== null && (
                <section className="mt-8" aria-labelledby="all-cvs-heading">
                  <h2 id="all-cvs-heading" className="mb-3 text-sm font-semibold text-gray-900">
                    All CVs
                  </h2>
                  <AllCvsList docs={docs} onDelete={deletion.requestDelete} />
                </section>
              )}
            </>
          )}
        </div>
        <aside className="w-full shrink-0 lg:sticky lg:top-6 lg:w-72">
          <PageTips page="cvs" orientation="vertical" />
        </aside>
      </div>
      {deletion.dialogs}
    </div>
  )
}

// A count as a small rounded pill in the page's colour; an empty text is the grey placeholder shown while the CVs are read, so no 0 flashes.
function CountPill({ text }: { text: string }) {
  if (text === '') {
    return (
      <span aria-hidden="true" data-testid="count-placeholder" className="inline-block h-5 w-14 animate-pulse rounded-full bg-gray-200" />
    )
  }
  return (
    <span
      className="inline-flex shrink-0 items-center rounded-full px-2.5 py-0.5 text-[11px] font-semibold"
      style={{ background: PAGE_ACCENT.soft, color: PAGE_ACCENT.text }}
    >
      {text}
    </span>
  )
}
