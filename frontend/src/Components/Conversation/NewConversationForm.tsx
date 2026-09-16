import { useState } from 'react'
import type { Platform } from '../../api/platforms'
import { Dropdown } from '../../Shared/Components/Dropdown/Dropdown'

// The one-time platform (and, for OpenAI/Anthropic, model) picker
// shown before a conversation exists. Originally local to
// ConversationPage.tsx; extracted (step 18) so GlobalChatWidget.tsx
// can share this exact same form instead of duplicating it — both
// call the shared ConversationContext's own createConversation with
// whatever this form's own onCreate hands back.
export function NewConversationForm({
  platforms,
  onCreate,
  onCancel,
}: {
  platforms: Platform[]
  onCreate: (input: { platformId: string; model?: string }) => void
  onCancel: () => void
}) {
  const [platformId, setPlatformId] = useState(platforms[0]?.id ?? '')
  const [model, setModel] = useState(platforms[0]?.models[0] ?? '')

  const selectedPlatform = platforms.find((p) => p.id === platformId)
  const needsModel = (selectedPlatform?.models.length ?? 0) > 0

  const handlePlatformChange = (id: string) => {
    setPlatformId(id)
    const next = platforms.find((p) => p.id === id)
    setModel(next?.models[0] ?? '')
  }

  const handleSubmit = (e: React.SyntheticEvent<HTMLFormElement>) => {
    e.preventDefault()
    onCreate({ platformId, model: needsModel ? model : undefined })
  }

  return (
    <div className="flex flex-1 items-center justify-center p-4">
      <form onSubmit={handleSubmit} className="w-full max-w-sm space-y-3 rounded-lg border border-gray-200 p-5">
        <h2 className="text-sm font-semibold text-gray-900">New conversation</h2>
        <div>
          {/* A span, not a <label> — Dropdown renders no native form
              control a label could actually be associated with. */}
          <span className="mb-1 block text-xs font-medium text-gray-500">Platform</span>
          <Dropdown options={platforms.map((p) => ({ value: p.id, label: p.name }))} value={platformId} onChange={handlePlatformChange} />
        </div>
        {needsModel && (
          <div>
            <span className="mb-1 block text-xs font-medium text-gray-500">Model</span>
            <Dropdown options={(selectedPlatform?.models ?? []).map((m) => ({ value: m, label: m }))} value={model} onChange={setModel} />
          </div>
        )}
        <p className="text-xs text-gray-400">The platform and model can&apos;t be changed after the conversation is created.</p>
        <div className="flex justify-end gap-2">
          <button
            type="button"
            onClick={onCancel}
            className="rounded border border-gray-300 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50"
          >
            Cancel
          </button>
          <button type="submit" className="rounded bg-gray-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-gray-800">
            Create
          </button>
        </div>
      </form>
    </div>
  )
}
