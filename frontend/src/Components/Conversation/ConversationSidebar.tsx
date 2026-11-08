import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import type { Conversation } from '../../api/conversations'
import type { Tool } from '../../api/tools'
import { useConfirm } from '../../Shared/Components/Modal/useConfirm'
import { IconButton } from '../../Shared/Components/IconButton/IconButton'
import { PencilIcon, TrashIcon, CheckIcon, XIcon, PlusIcon, LogsIcon } from '../../Shared/Components/IconButton/icons'
import { Dropdown, type DropdownOption } from '../../Shared/Components/Dropdown/Dropdown'
import { TurnStatusBadge } from './TurnStatusBadge'

// NO_TOOL_FILTER is a reserved filter value, never a real installed
// tool's own slug (every real one is a plain manifest-declared
// identifier like "career" — this app has no double-underscore-wrapped
// naming convention for those) — picked so "show only conversations
// NOT tied to any tool" fits in the same single toolFilter value as
// "show only this tool's own conversations," rather than needing a
// second boolean alongside it.
const NO_TOOL_FILTER = '__no_tool__'

// List, "New conversation", inline rename, delete-behind-confirm — same
// established pattern ToolsListPage/PlatformKeysPage already use. See
// plan/ai/conversation/step-05-frontend-chat-ui.md.
//
// Search-by-title and filter-by-tool (step 41) are both plain
// client-side filters over the already-fully-loaded `conversations`
// prop — this list has no pagination and is already polled in full by
// the shared context (its own activeTurn badges need every item live),
// so there's no round trip to save by pushing either filter to the
// backend. Deliberately titled/labeled as a TITLE search, never a
// prompt/message search — a conversation's own message content lives
// in a separate log file this list never loads at all, so searching it
// here isn't even possible without a very different, heavier feature;
// see plan/ai/conversation/step-41-search-and-filter-by-tool.md.
export function ConversationSidebar({
  conversations,
  tools,
  selectedId,
  busyId,
  onSelect,
  onCreate,
  onRename,
  onDelete,
}: {
  conversations: Conversation[]
  tools: Tool[]
  selectedId: string | null
  busyId: string | null
  onSelect: (id: string) => void
  onCreate: () => void
  onRename: (id: string, title: string) => void
  onDelete: (id: string) => void
}) {
  const [renamingId, setRenamingId] = useState<string | null>(null)
  const [renameValue, setRenameValue] = useState('')
  const [titleQuery, setTitleQuery] = useState('')
  const [toolFilter, setToolFilter] = useState('')
  const { confirm, dialog } = useConfirm()
  const navigate = useNavigate()

  const toolFilterOptions: DropdownOption[] = [
    { value: '', label: 'All tools' },
    { value: NO_TOOL_FILTER, label: 'Not tool-specific' },
    ...tools.map((t) => ({ value: t.slug, label: t.name })),
  ]

  const filteredConversations = useMemo(() => {
    const q = titleQuery.trim().toLowerCase()
    return conversations.filter((c) => {
      if (q && !c.title.toLowerCase().includes(q)) return false
      if (toolFilter === NO_TOOL_FILTER && c.toolSlug) return false
      if (toolFilter && toolFilter !== NO_TOOL_FILTER && c.toolSlug !== toolFilter) return false
      return true
    })
  }, [conversations, titleQuery, toolFilter])

  const startRename = (c: Conversation) => {
    setRenamingId(c.id)
    setRenameValue(c.title)
  }

  const commitRename = (id: string) => {
    const title = renameValue.trim()
    setRenamingId(null)
    if (title === '') return
    onRename(id, title)
  }

  const cancelRename = () => {
    setRenamingId(null)
  }

  // Stops the rename <input>'s own onBlur from firing (and committing)
  // before Save/Cancel's onClick runs — a click on either button would
  // otherwise blur the input first, silently committing even a Cancel
  // click. See plan/ai/conversation/step-10-sidebar-action-icons.md.
  const preventBlur = (e: React.MouseEvent) => {
    e.preventDefault()
  }

  const handleDelete = async (c: Conversation) => {
    const confirmed = await confirm({
      title: 'Delete conversation',
      message: `Delete "${c.title}"? This cannot be undone.`,
    })
    if (!confirmed) return
    onDelete(c.id)
  }

  return (
    <div className="flex h-full w-64 shrink-0 flex-col border-r border-gray-200">
      <div className="flex items-center justify-between border-b border-gray-100 px-3 py-2">
        <span className="text-sm font-medium text-gray-400">Conversation</span>
        <IconButton icon={<PlusIcon />} label="New conversation" onClick={onCreate} />
      </div>

      <div className="flex flex-col gap-1.5 border-b border-gray-100 px-3 py-2">
        <input
          type="text"
          value={titleQuery}
          onChange={(e) => {
            setTitleQuery(e.target.value)
          }}
          placeholder="Search titles…"
          aria-label="Search conversation titles"
          className="w-full rounded border border-gray-300 px-2 py-1 text-sm focus:border-gray-500 focus:outline-none"
        />
        {/* Explicit, so it's never mistaken for a search over message
            content — this only ever matches each conversation's own
            title, never anything said inside it. */}
        <p className="text-xs text-gray-400">Searches titles only, not message content</p>
        {tools.length > 0 && (
          <Dropdown options={toolFilterOptions} value={toolFilter} onChange={setToolFilter} placeholder="All tools" />
        )}
      </div>

      <div className="flex-1 overflow-y-auto">
        {conversations.length === 0 && <p className="px-3 py-6 text-center text-sm text-gray-400">No conversations yet.</p>}
        {conversations.length > 0 && filteredConversations.length === 0 && (
          <p className="px-3 py-6 text-center text-sm text-gray-400">No conversations match your search.</p>
        )}
        <ul>
          {filteredConversations.map((c) => (
            <li
              key={c.id}
              className={`group flex items-center gap-1 border-b border-gray-100 px-3 py-2 text-sm ${
                c.id === selectedId ? 'bg-gray-100' : 'hover:bg-gray-50'
              }`}
            >
              {renamingId === c.id ? (
                <input
                  // Deliberate: entering rename mode should focus the
                  // input immediately, the same convention GitHub/most
                  // inline-rename UIs use.
                  // eslint-disable-next-line jsx-a11y/no-autofocus
                  autoFocus
                  value={renameValue}
                  onChange={(e) => {
                    setRenameValue(e.target.value)
                  }}
                  onBlur={() => {
                    commitRename(c.id)
                  }}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') commitRename(c.id)
                    if (e.key === 'Escape') cancelRename()
                  }}
                  className="min-w-0 flex-1 rounded border border-gray-300 px-1 py-0.5 text-sm"
                />
              ) : (
                <button
                  type="button"
                  onClick={() => {
                    onSelect(c.id)
                  }}
                  onDoubleClick={() => {
                    startRename(c)
                  }}
                  disabled={busyId === c.id}
                  className="min-w-0 flex-1 text-left text-gray-800 disabled:opacity-50"
                  title={c.title}
                >
                  <span className="block truncate">{c.title}</span>
                  <TurnStatusBadge activeTurn={c.activeTurn} />
                </button>
              )}
              {renamingId === c.id ? (
                <div className="flex shrink-0 gap-0.5">
                  <IconButton
                    icon={<CheckIcon />}
                    label="Save"
                    onMouseDown={preventBlur}
                    onClick={() => {
                      commitRename(c.id)
                    }}
                  />
                  <IconButton icon={<XIcon />} label="Cancel" onMouseDown={preventBlur} onClick={cancelRename} />
                </div>
              ) : (
                <div className="flex shrink-0 gap-0.5 opacity-0 group-hover:opacity-100">
                  <IconButton
                    icon={<LogsIcon />}
                    label="View AI logs"
                    onClick={() => {
                      void navigate(`/conversations/${c.id}/logs`)
                    }}
                  />
                  <IconButton
                    icon={<PencilIcon />}
                    label="Rename"
                    onClick={() => {
                      startRename(c)
                    }}
                    disabled={busyId === c.id}
                  />
                  <IconButton
                    icon={<TrashIcon />}
                    label="Delete"
                    onClick={() => {
                      void handleDelete(c)
                    }}
                    disabled={busyId === c.id}
                    variant="danger"
                  />
                </div>
              )}
            </li>
          ))}
        </ul>
      </div>
      {dialog}
    </div>
  )
}
