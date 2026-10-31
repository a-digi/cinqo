import { mediaDownloadUrl, type CvDocument, type CvTemplate } from '../../api'
import { SparkleIcon } from '../../Shared/Icons/icons'
import { CV_BUILDER_PATH } from '../../paths'
import { createdLabel, templateLabel } from './cvCardRules'

// The rows of a read-only list of CVs, shared by the profile and persona view pages: title, the "Created
// with AI" mark for a CV an AI run made (aiCvIds holds those CVs' ids), template and date, the personas
// the CV is built from or attached to, and PDF and Edit links. The page around it (heading, empty
// text, warnings) is the page's own.
export function CvList({ docs, templates, aiCvIds }: { docs: CvDocument[]; templates: CvTemplate[]; aiCvIds: ReadonlySet<string> }) {
  return (
    <ul className="space-y-2">
      {docs.map((doc) => (
        <li
          key={doc.id}
          className="flex flex-wrap items-center gap-x-2 gap-y-1 rounded-2xl border border-gray-100 bg-white/70 p-3 text-xs text-gray-600"
        >
          <span className="min-w-0 max-w-full truncate text-sm font-medium text-gray-900" title={doc.title}>
            {doc.title}
          </span>
          {aiCvIds.has(doc.id) && (
            <span className="inline-flex items-center gap-1 rounded-full bg-purple-50 px-2 py-0.5 text-[11px] font-medium text-purple-700">
              <SparkleIcon />
              Created with AI
            </span>
          )}
          <span className="text-gray-400">
            {[templateLabel(templates, doc.templateId), createdLabel(doc.createdAt)].filter((x) => x).join(' · ')}
          </span>
          <span className="flex flex-wrap gap-1">
            {doc.personas.map((cp) => (
              <span
                key={cp.id}
                title={cp.builtFrom ? 'Built from this persona' : 'Attached persona'}
                className={`rounded-full px-2 py-0.5 text-[10px] font-medium ${cp.builtFrom ? 'bg-purple-100 text-purple-800' : 'bg-gray-100 text-gray-700'}`}
              >
                {cp.name}
              </span>
            ))}
          </span>
          <span className="ml-auto flex gap-3">
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
                window.__cinqoToolBridge.navigate(`${CV_BUILDER_PATH}?id=${encodeURIComponent(doc.id)}`)
              }}
              className="text-blue-700 hover:underline"
            >
              Edit
            </button>
          </span>
        </li>
      ))}
    </ul>
  )
}
