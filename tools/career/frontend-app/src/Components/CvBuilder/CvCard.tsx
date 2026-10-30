import { ActionMenu } from '../../Shared/ActionMenu/ActionMenu'
import { EyeIcon, PencilIcon, TrashIcon } from '../../Shared/Icons/icons'
import { PAGE_ACCENT } from '../JobsPage/jobsTheme'
import { CvPreview } from './CvPreview'
import { createdLabel } from './cvCardRules'
import type { CvDocument } from '../../api'

const floatingButton =
  'flex h-7 w-7 items-center justify-center rounded-lg bg-white text-gray-700 shadow transition-colors hover:bg-blue-50 hover:text-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500'

// One CV document as a card, the way the Media page shows a file: the preview on top (a portrait
// page, lifted a little on hover), the title and a few facts under it. Preview, Edit and a "more"
// menu (Rename, Delete) fade in on hover or focus, and stay visible on a touch screen. Clicking the
// preview opens the editor.
export function CvCard({
  doc,
  templateName,
  onPreview,
  onEdit,
  onRename,
  onDelete,
}: {
  doc: CvDocument
  templateName: string
  onPreview: () => void
  onEdit: () => void
  onRename: () => void
  onDelete: () => void
}) {
  const controls = 'opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 [@media(hover:none)]:opacity-100'
  const created = createdLabel(doc.createdAt)
  return (
    <div className="group min-w-0">
      <div className="relative aspect-[3/4] overflow-hidden rounded-xl bg-gray-100 shadow-sm transition duration-200 group-hover:-translate-y-0.5 group-hover:shadow-md">
        <button type="button" onClick={onEdit} aria-label={`Edit ${doc.title}`} className="block h-full w-full cursor-pointer">
          <CvPreview mediaFileId={doc.mediaFileId} templateId={doc.templateId} version={doc.updatedAt ?? doc.createdAt} title={doc.title} />
        </button>
        {doc.jobId && (
          <span
            className="absolute left-2 top-2 rounded-full px-2 py-0.5 text-[10px] font-semibold shadow-sm"
            style={{ background: PAGE_ACCENT.soft, color: PAGE_ACCENT.text }}
          >
            For a job
          </span>
        )}
        <div className={`absolute right-2 top-2 flex items-center gap-1.5 transition-opacity ${controls}`}>
          <button
            type="button"
            onClick={onPreview}
            aria-label={`Preview ${doc.title} (PDF)`}
            title="Preview the PDF"
            className={floatingButton}
          >
            <EyeIcon />
          </button>
          <button type="button" onClick={onEdit} aria-label={`Edit ${doc.title}`} title="Edit" className={floatingButton}>
            <PencilIcon />
          </button>
          <div className="rounded-lg bg-white shadow">
            <ActionMenu
              triggerLabel={`More actions for ${doc.title}`}
              items={[
                { key: 'rename', label: 'Rename', onClick: onRename },
                { key: 'delete', label: 'Delete', icon: <TrashIcon />, variant: 'danger', onClick: onDelete },
              ]}
            />
          </div>
        </div>
      </div>
      <p className="mt-2 line-clamp-2 break-words text-sm font-medium text-gray-900" title={doc.title}>
        {doc.title}
      </p>
      <div className="mt-1 flex flex-wrap items-center gap-1.5 text-xs text-gray-500">
        <span className="rounded-full px-2 py-0.5 font-medium" style={{ background: PAGE_ACCENT.soft, color: PAGE_ACCENT.text }}>
          {templateName}
        </span>
        {created && <span>{created}</span>}
      </div>
    </div>
  )
}
