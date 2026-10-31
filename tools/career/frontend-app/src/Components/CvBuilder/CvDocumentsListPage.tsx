import { useEffect, useState } from 'react'
import { fetchAllCvDocuments, fetchPersonas, fetchProfiles, type CvDocument, type Persona, type Profile } from '../../api'
import { PDFIcon } from '../../Shared/Icons/icons'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { PageTips } from '../../Shared/PageTips/PageTips'
import { PERSONAS_PATH, cvDocumentsPathFor } from '../../paths'
import { plural } from '../CompaniesPage/companyStats'
import { PersonaList } from '../Persona/PersonaList'
import { previewItems } from '../Persona/personaListRules'
import { cvListMayBeCut } from '../ProfilesPage/profileViewRules'
import { cvsOfPersona } from './cvPersonaRules'

// How many of a persona's CV titles the list shows before "+N more".
const PREVIEW_CVS = 3

// The CVs page: the personas as a list (grouped by profile), each with how many CVs it has (built from it or
// attached to it) and the first few titles; a click opens that persona's own CV page
// (/tools/career/cv-documents/:personaId), where the CVs are shown as cards and created. Nothing is edited here,
// and there is no dropdown. See plan/ai/tools/career/step-122-cv-documents-per-persona-pages.md.
export function CvDocumentsListPage() {
  const [personas, setPersonas] = useState<Persona[]>([])
  const [profiles, setProfiles] = useState<Profile[]>([])
  // Every CV (with its personas); null when they could not be read, so no count is shown.
  const [docs, setDocs] = useState<CvDocument[] | null>(null)
  const [error, setError] = useState('')
  const [loaded, setLoaded] = useState(false)

  useEffect(() => {
    Promise.all([fetchPersonas(), fetchProfiles()])
      .then(([personaList, profileList]) => {
        setPersonas(personaList)
        setProfiles(profileList)
        fetchAllCvDocuments()
          .then(setDocs)
          .catch(() => {
            setDocs(null)
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
        description="Pick a persona to see its CVs and create new ones from its own data, using one of a few fixed designs."
        pill={loaded && error === '' ? { text: plural(personas.length, 'persona', 'personas') } : undefined}
      />
      {/* The tips sit in a column of their own beside the list (under it on a narrow window). */}
      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <div className="order-1 min-w-0 flex-1">
          {error !== '' && <p className="mb-3 text-sm text-red-700">{error}</p>}
          {loaded && error === '' && personas.length === 0 && (
            <p className="rounded-3xl border border-gray-100 bg-gray-50 p-8 text-center text-sm text-gray-500">
              There are no personas yet.{' '}
              <a
                href={PERSONAS_PATH}
                onClick={(e) => {
                  e.preventDefault()
                  window.__cinqoToolBridge.navigate(PERSONAS_PATH)
                }}
                className="font-medium text-blue-700 hover:underline"
              >
                Create one on the Personas page
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
              <PersonaList
                personas={personas}
                profiles={profiles}
                hrefFor={(p) => cvDocumentsPathFor(p.id)}
                renderExtra={(p) => {
                  if (docs === null) return null
                  const mine = cvsOfPersona(docs, p.id)
                  const { shown, more } = previewItems(
                    mine.map((m) => m.doc.title),
                    PREVIEW_CVS,
                  )
                  return (
                    <span className="mt-1.5 block text-xs text-gray-500">
                      <span className="font-medium text-gray-700">{plural(mine.length, 'CV', 'CVs')}</span>
                      {shown.length > 0 && <span> · {shown.join(' · ')}</span>}
                      {more > 0 && <span> · +{more} more</span>}
                    </span>
                  )
                }}
              />
            </>
          )}
        </div>
        <aside className="w-full shrink-0 lg:sticky lg:top-6 lg:w-72">
          <PageTips page="cvs" orientation="vertical" />
        </aside>
      </div>
    </div>
  )
}
