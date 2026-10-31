import type { CvDocument, CvTemplate } from '../../../api'
import { mediaDownloadUrl } from '../../../api'
import { CV_BUILDER_PATH, CV_DOCUMENTS_PATH } from '../../../paths'
import { createdLabel, templateLabel } from '../../CvBuilder/cvCardRules'
import { cvsOfPersona } from '../../CvBuilder/cvPersonaRules'
import { setPersonaIdInLocalStorage } from '../PersonaSwitcher/repositoryLocalStorage'

// The CVs linked to one persona (step 114), under its card on the Personas page: the ones built
// from it and the ones attached to it. A row opens the PDF or the editor; the footer opens the CV
// Documents page on this persona, where CVs are attached and detached.
export function PersonaCvs({ personaId, docs, templates }: { personaId: string; docs: CvDocument[]; templates: CvTemplate[] }) {
  const rows = cvsOfPersona(docs, personaId)
  const open = (path: string) => {
    window.__cinqoToolBridge.navigate(path)
  }
  return (
    <section aria-label="CVs of this persona" className="mt-3 border-t border-gray-100 pt-2">
      <p className="mb-1 text-[11px] font-semibold uppercase tracking-wider text-gray-400">
        {rows.length === 1 ? '1 CV' : `${rows.length} CVs`}
      </p>
      {rows.length > 0 && (
        <ul className="space-y-1">
          {rows.map(({ doc, builtFrom }) => {
            const created = createdLabel(doc.createdAt)
            return (
              <li key={doc.id} className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-gray-600">
                <span className="min-w-0 max-w-full truncate font-medium text-gray-900" title={doc.title}>
                  {doc.title}
                </span>
                <span
                  title={builtFrom ? 'Built from this persona' : 'Attached to this persona'}
                  className={`rounded-full px-2 py-0.5 text-[10px] font-medium ${
                    builtFrom ? 'bg-purple-100 text-purple-800' : 'bg-gray-100 text-gray-700'
                  }`}
                >
                  {builtFrom ? 'Built from this persona' : 'Attached'}
                </span>
                <span className="text-gray-400">{[templateLabel(templates, doc.templateId), created].filter((x) => x).join(' · ')}</span>
                <span className="ml-auto flex gap-2">
                  {doc.mediaFileId !== '' && (
                    <a
                      href={mediaDownloadUrl(doc.mediaFileId)}
                      target="_blank"
                      rel="noreferrer"
                      className="text-gray-600 underline hover:text-gray-900"
                    >
                      PDF
                    </a>
                  )}
                  <button
                    type="button"
                    onClick={() => {
                      open(`${CV_BUILDER_PATH}?id=${encodeURIComponent(doc.id)}`)
                    }}
                    className="text-blue-700 hover:underline"
                  >
                    Edit
                  </button>
                </span>
              </li>
            )
          })}
        </ul>
      )}
      <button
        type="button"
        onClick={() => {
          setPersonaIdInLocalStorage(personaId)
          open(rows.length > 0 ? CV_DOCUMENTS_PATH : `${CV_BUILDER_PATH}?personaId=${encodeURIComponent(personaId)}`)
        }}
        className="mt-1.5 text-xs text-blue-700 hover:underline"
      >
        {rows.length > 0 ? 'Manage CVs' : 'Create a CV'}
      </button>
    </section>
  )
}
