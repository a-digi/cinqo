import type { CvDocument } from '../../api'
import { CV_BUILDER_PATH } from '../../paths'
import { allCvRows, rowFacts, NO_CVS_TEXT } from './allCvsRules'

// Every CV in one list with a Delete that is always visible (not only on hover), under the personas on the CV Documents page. Opening a
// CV goes to the editor, the way a card on a persona's page does. The delete itself (its confirmations and what it does) is the shared
// flow the page gives in `onDelete`. See plan/ai/tools/career/cv-delete-from-list.md, step 3.
export function AllCvsList({ docs, onDelete }: { docs: CvDocument[]; onDelete: (doc: CvDocument) => void }) {
  const rows = allCvRows(docs)
  const byId = new Map(docs.map((d) => [d.id, d]))

  if (rows.length === 0) {
    return <p className="rounded-3xl border border-gray-100 bg-gray-50 p-6 text-center text-sm text-gray-500">{NO_CVS_TEXT}</p>
  }

  return (
    <ul className="divide-y divide-gray-100 overflow-hidden rounded-2xl border border-gray-100 bg-white" aria-label="All CVs">
      {rows.map((row) => {
        const doc = byId.get(row.id)
        const facts = rowFacts(row)
        return (
          <li key={row.id} className="flex items-center gap-3 px-4 py-3">
            <div className="min-w-0 flex-1">
              <button
                type="button"
                onClick={() => {
                  window.__cinqoToolBridge.navigate(`${CV_BUILDER_PATH}?id=${encodeURIComponent(row.id)}`)
                }}
                className="block max-w-full truncate text-left text-sm font-medium text-gray-900 hover:text-blue-700 hover:underline"
                title="Open this CV"
              >
                {row.title}
              </button>
              {facts !== '' && <span className="mt-0.5 block truncate text-xs text-gray-500">{facts}</span>}
            </div>
            <button
              type="button"
              onClick={() => {
                if (doc) onDelete(doc)
              }}
              aria-label={`Delete ${row.title}`}
              className="shrink-0 rounded-md border border-red-200 px-3 py-1.5 text-xs font-medium text-red-700 hover:bg-red-50"
            >
              Delete
            </button>
          </li>
        )
      })}
    </ul>
  )
}
