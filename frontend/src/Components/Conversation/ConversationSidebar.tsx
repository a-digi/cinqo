import { useState } from 'react'
import type { Conversation } from '../../api/conversations'
import { useConfirm } from '../../Shared/Components/Modal/useConfirm'

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
      <div className="p-3">
        <button
          type="button"
          onClick={onCreate}
          className="w-full rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white hover:bg-gray-800"
        >
          New conversation
        </button>
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
                  autoFocus
                  value={renameValue}
                  onChange={(e) => setRenameValue(e.target.value)}
                  onBlur={() => commitRename(c.id)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') commitRename(c.id)
                    if (e.key === 'Escape') setRenamingId(null)
                  }}
                  className="min-w-0 flex-1 rounded border border-gray-300 px-1 py-0.5 text-sm"
                />
              ) : (
                <button
                  type="button"
                  onClick={() => onSelect(c.id)}
                  onDoubleClick={() => startRename(c)}
                  disabled={busyId === c.id}
                  className="min-w-0 flex-1 truncate text-left text-gray-800 disabled:opacity-50"
                  title={c.title}
                >
                  {c.title}
                </button>
              )}
              <button
                type="button"
                onClick={() => handleDelete(c)}
                disabled={busyId === c.id}
                className="shrink-0 text-xs font-medium text-red-600 opacity-0 hover:text-red-800 disabled:opacity-50 group-hover:opacity-100"
              >
                Delete
              </button>
            </li>
          ))}
        </ul>
      </div>
      {dialog}
    </div>
  )
}
