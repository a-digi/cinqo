import { useId, useState } from 'react'
import { Link } from 'react-router-dom'
import { useAuth } from '../Auth/AuthContext'
import { canAddKeys } from './canAddKeys'
import type { OrchestrationMode } from '../../api/conversations'
import { selectablePlatforms, type Platform } from '../../api/platforms'
import { Dropdown } from '../../Shared/Components/Dropdown/Dropdown'
import { useKeepPlatformsFresh } from '../../config/platforms/useKeepPlatformsFresh'

// The one-time platform (and, for OpenAI/Anthropic, model) picker
// shown before a conversation exists. Originally local to
// ConversationPage.tsx; extracted (step 18) so GlobalChatWidget.tsx
// can share this exact same form instead of duplicating it — both
// call the shared ConversationContext's own createConversation with
// whatever this form's own onCreate hands back.
// Takes every platform and offers only those with a key (or that need none), so the page and the chat window cannot differ
// (plan/ai/conversation/step-229-platforms-with-a-key-only.md). With none to offer it says so instead of showing the form.
export function NewConversationForm({
  platforms: all,
  onCreate,
  onCancel,
}: {
  platforms: Platform[]
  onCreate: (input: { platformId: string; model?: string; orchestrationMode: OrchestrationMode }) => void
  onCancel: () => void
}) {
  const platforms = selectablePlatforms(all)
  if (platforms.length === 0) return <NoPlatformWithAKey onCancel={onCancel} />
  return <Form platforms={platforms} onCreate={onCreate} onCancel={onCancel} />
}

function NoPlatformWithAKey({ onCancel }: { onCancel: () => void }) {
  const { scopes } = useAuth()
  // Nothing can be started yet: look again every few seconds, so a key or a local model added meanwhile turns this into the form by itself.
  useKeepPlatformsFresh()
  return (
    <div className="flex flex-1 items-center justify-center p-4">
      <div className="w-full max-w-sm space-y-3 rounded-lg border border-gray-200 p-5 text-center">
        <h2 className="text-sm font-semibold text-gray-900">New conversation</h2>
        <p className="text-sm text-gray-600">No AI platform has an API key yet.</p>
        {canAddKeys(scopes) && (
          <Link to="/admin/api-keys" className="block text-sm font-medium text-gray-900 underline">
            Add an API key
          </Link>
        )}
        <button
          type="button"
          onClick={onCancel}
          className="rounded border border-gray-300 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50"
        >
          Back
        </button>
      </div>
    </div>
  )
}

function Form({
  platforms,
  onCreate,
  onCancel,
}: {
  platforms: Platform[]
  onCreate: (input: { platformId: string; model?: string; orchestrationMode: OrchestrationMode }) => void
  onCancel: () => void
}) {
  const [platformId, setPlatformId] = useState(platforms[0]?.id ?? '')
  const [model, setModel] = useState(platforms[0]?.models[0] ?? '')
  // Composer mode (composer step 7): the AI plans and sub-agents do the
  // work, instead of the AI using tools itself. On by default; fixed
  // for the conversation's lifetime like the platform and model.
  const [composer, setComposer] = useState(true)
  const composerId = useId()

  const selectedPlatform = platforms.find((p) => p.id === platformId)
  const needsModel = (selectedPlatform?.models.length ?? 0) > 0
  // A platform that offers no tools (the local AI) can only answer directly: composer mode is off and cannot be chosen.
  const offersTools = selectedPlatform?.offersTools ?? true
  const composerOn = composer && offersTools

  const handlePlatformChange = (id: string) => {
    setPlatformId(id)
    const next = platforms.find((p) => p.id === id)
    setModel(next?.models[0] ?? '')
  }

  const handleSubmit = (e: React.SyntheticEvent<HTMLFormElement>) => {
    e.preventDefault()
    onCreate({ platformId, model: needsModel ? model : undefined, orchestrationMode: composerOn ? 'composer' : 'direct' })
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
        <div className="flex items-start gap-2 text-xs text-gray-600">
          <input
            id={composerId}
            type="checkbox"
            checked={composerOn}
            disabled={!offersTools}
            onChange={(e) => {
              setComposer(e.target.checked)
            }}
            className="mt-0.5"
          />
          <label htmlFor={composerId}>
            <span className="font-medium text-gray-700">Delegate work to sub-agents</span>
            <span className="block text-gray-400">
              {offersTools
                ? 'The AI plans and sub-agents do the work with the installed tools. Turn off to let the AI use the tools itself.'
                : 'This platform offers no tools, so the AI only answers.'}
            </span>
          </label>
        </div>
        <p className="text-xs text-gray-400">
          The platform, model and this setting can&apos;t be changed after the conversation is created.
        </p>
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
