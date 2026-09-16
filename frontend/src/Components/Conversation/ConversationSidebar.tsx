import { useState } from 'react'
import type { Conversation } from '../../api/conversations'
import { useConfirm } from '../../Shared/Components/Modal/useConfirm'
import { IconButton } from '../../Shared/Components/IconButton/IconButton'
import { PencilIcon, TrashIcon, CheckIcon, XIcon, PlusIcon } from '../../Shared/Components/IconButton/icons'
import { TurnStatusBadge } from './TurnStatusBadge'

// List, "New conversation", inline rename, delete-behind-confirm — same
// established pattern ToolsListPage/PlatformKeysPage already use. See
// plan/ai/conversation/step-05-frontend-chat-ui.md.
export function ConversationSidebar({
  conversations,
  selectedId,
  busyId,
  onSelect,
  onCreate,
  onRename,
  onDelete,
}: {
  conversations: Conversation[]
  selectedId: string | null
  busyId: string | null
  onSelect: (id: string) => void
  onCreate: () => void
  onRename: (id: string, title: string) => void
  onDelete: (id: string) => void
}) {
  const [renamingId, setRenamingId] = useState<string | null>(null)
  const [renameValue, setRenameValue] = useState('')
  const { confirm, dialog } = useConfirm()

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

      <div className="flex-1 overflow-y-auto">
        {conversations.length === 0 && <p className="px-3 py-6 text-center text-sm text-gray-400">No conversations yet.</p>}
        <ul>
          {conversations.map((c) => (
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
