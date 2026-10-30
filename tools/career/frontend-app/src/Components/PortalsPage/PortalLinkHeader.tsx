import type { PortalLink } from '../../api'
import { LinkStatusPills } from '../../Crawler/LinkStatusPills'

interface PortalLinkHeaderProps {
  link: PortalLink
  isEditing: boolean
  editTitle: string
  editUrl: string
  onEditTitleChange: (value: string) => void
  onEditUrlChange: (value: string) => void
  onStartEdit: () => void
  onSaveEdit: () => void
  onCancelEdit: () => void
  onRemove: () => void
}

// PortalLinkHeader (step 89) — what a portal link's accordion header
// shows: title (opens the page in a new tab), URL, last-crawled line, the
// status pills, and Edit/Remove — or, while editing, the inline
// title/URL inputs with Save/Cancel. Extracted from PortalsPage's inline
// JSX. Lives inside AccordionItem's clickable header row, so every
// control here stops click propagation (and the edit form key events)
// the way the portal header already does — otherwise using it would
// also toggle the accordion. See
// plan/ai/tools/career/step-89-portal-link-accordion.md.
export function PortalLinkHeader({
  link,
  isEditing,
  editTitle,
  editUrl,
  onEditTitleChange,
  onEditUrlChange,
  onStartEdit,
  onSaveEdit,
  onCancelEdit,
  onRemove,
}: PortalLinkHeaderProps) {
  if (isEditing) {
    return (
      // Not a real interaction — only keeps the header row's own
      // click/keydown toggle from firing while editing.
      // eslint-disable-next-line jsx-a11y/no-static-element-interactions
      <div
        className="space-y-1.5"
        onClick={(e) => {
          e.stopPropagation()
        }}
        onKeyDown={(e) => {
          e.stopPropagation()
        }}
      >
        <input
          // Deliberate: entering edit mode should focus the input
          // immediately, the same convention most inline-rename UIs use.
          // eslint-disable-next-line jsx-a11y/no-autofocus
          autoFocus
          value={editTitle}
          onChange={(e) => {
            onEditTitleChange(e.target.value)
          }}
          placeholder="Title, e.g. Software Engineer jobs, Hamburg"
          className="w-full rounded-md border border-gray-300 px-2 py-1 text-xs text-gray-900 focus:border-gray-500 focus:outline-none"
        />
        <input
          value={editUrl}
          onChange={(e) => {
            onEditUrlChange(e.target.value)
          }}
          placeholder="URL"
          className="w-full rounded-md border border-gray-300 px-2 py-1 text-xs text-gray-900 focus:border-gray-500 focus:outline-none"
        />
        <div className="flex gap-2">
          <button
            type="button"
            onClick={onSaveEdit}
            className="rounded-md bg-gray-900 px-2.5 py-1 text-xs font-medium text-white hover:bg-gray-800"
          >
            Save
          </button>
          <button
            type="button"
            onClick={onCancelEdit}
            className="rounded-md border border-gray-200 px-2.5 py-1 text-xs text-gray-700 hover:bg-white"
          >
            Cancel
          </button>
        </div>
      </div>
    )
  }

  return (
    <div className="flex items-start justify-between gap-2">
      <div className="min-w-0">
        <a
          href={link.url}
          target="_blank"
          rel="noopener noreferrer"
          onClick={(e) => {
            e.stopPropagation()
          }}
          className="block truncate text-xs font-semibold text-blue-700 underline decoration-blue-200 hover:decoration-blue-500"
        >
          {link.title ?? '(untitled link)'}
        </a>
        <div className="truncate text-xs text-gray-500">{link.url}</div>
        <div className="truncate text-xs text-gray-400">
          {link.lastCrawledAt ? `Last crawled: ${new Date(link.lastCrawledAt).toLocaleString()}` : 'Never crawled'}
        </div>
        <LinkStatusPills link={link} showCrawling className="mt-1.5" />
      </div>
      <div className="flex shrink-0 gap-2">
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation()
            onStartEdit()
          }}
          className="rounded-full border border-blue-200 bg-white px-2.5 py-0.5 text-xs text-blue-700 hover:bg-blue-50"
        >
          Edit
        </button>
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation()
            onRemove()
          }}
          className="rounded-full border border-red-200 bg-white px-2.5 py-0.5 text-xs text-red-700 hover:bg-red-50"
        >
          Remove
        </button>
      </div>
    </div>
  )
}
