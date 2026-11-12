import { useEffect, useRef, useState } from 'react'
import { resolveAiOverride, type AiOverride } from '../../../Cinqo/Platform/aiOverride'
import { usesCodeDriver } from '../../../Cinqo/Platform/modelLabels'
import { useAiChoice } from '../../../Cinqo/Platform/useAiChoice'
import { CvAiPicker } from '../../../CvAi/CvAiPicker'
import { SETTINGS_PATH } from '../../../paths'
import { Modal } from '../../../Shared/Modal/Modal'
import { SparkleIcon } from '../../../Shared/Icons/icons'
import { startResuggestChat, type ResuggestChat } from './startResuggestChat'
import {
  CODE_DRIVER_MESSAGE,
  DEFAULT_WISH,
  MAX_WISH_LENGTH,
  SECTION_TITLES,
  scopeChip,
  scopeTitle,
  turnFailure,
  wishOf,
  type ResuggestLine,
  type ResuggestScope,
} from './resuggestRules'

// The small chat about one section or some rows of the suggestions of a CV import (plan 268). The AI is one the person picks for this chat (it starts from the Career AI settings
// and is not saved). The first message creates a hidden conversation bound to the scope; later messages continue it. Nothing is stored by the dialog: closing it forgets the
// transcript, and the suggestions themselves (changed by the AI's tool) are what the review page shows. `onTurnEnd` lets the page look at the run at once; `onBusyChange` tells it
// a turn is running (Insert is off meanwhile).
export function ResuggestDialog({
  run,
  scope,
  onClose,
  onTurnEnd,
  onBusyChange,
}: {
  run: { id: string; mediaFileId: string; originalFilename: string }
  scope: ResuggestScope
  onClose: () => void
  onTurnEnd: () => Promise<void> | void
  onBusyChange: (busy: boolean) => void
}) {
  const ai = useAiChoice()
  const [override, setOverride] = useState<AiOverride | null>(null)
  const choice = resolveAiOverride({ platform: ai.platform, model: ai.model }, ai.platforms, override)
  const [text, setText] = useState('')
  const [lines, setLines] = useState<ResuggestLine[]>([])
  const [busy, setBusy] = useState(false)
  // The chat exists from the first send on; its AI is then fixed.
  const [started, setStarted] = useState<{ name: string } | null>(null)
  const chat = useRef<ResuggestChat | null>(null)
  const firstSent = useRef(false)

  useEffect(() => {
    onBusyChange(busy)
  }, [busy, onBusyChange])
  useEffect(
    () => () => {
      onBusyChange(false)
    },
    [onBusyChange],
  )

  function add(line: ResuggestLine) {
    setLines((prev) => [...prev, line])
  }

  async function send() {
    if (busy) return
    const platform = choice.platform
    if (!platform) {
      add({ role: 'error', text: 'No AI platform is available. Choose one in Settings.' })
      return
    }
    if (chat.current === null && usesCodeDriver(platform, choice.model)) {
      add({ role: 'error', text: CODE_DRIVER_MESSAGE })
      return
    }
    const wish = wishOf(text)
    add({ role: 'user', text: text.trim() === '' ? DEFAULT_WISH : text.trim() })
    setText('')
    setBusy(true)
    try {
      if (chat.current === null) {
        chat.current = await startResuggestChat(run, scope, { platform, model: choice.model, source: ai.source })
        setStarted({ name: `${platform.name}${choice.model ? ` · ${choice.model}` : ''}` })
      }
      const wasFirst = !firstSent.current
      const reply = wasFirst ? await chat.current.first(wish) : await chat.current.next(wish)
      firstSent.current = true
      add({ role: 'ai', text: reply.content.trim() !== '' ? reply.content.trim() : 'The AI made no reply.' })
    } catch (err) {
      add({ role: 'error', text: turnFailure(err) })
    }
    try {
      await onTurnEnd()
    } catch {
      // the page looks again at its next poll
    }
    setBusy(false)
  }

  const aiText = started?.name ?? (choice.platform ? `${choice.platform.name}${choice.model ? ` · ${choice.model}` : ''}` : '')

  return (
    <Modal
      open
      title={scopeTitle(scope)}
      onClose={() => {
        if (!busy) onClose()
      }}
    >
      <p className="mb-3 text-sm text-gray-600">
        The AI looks at the <span className="font-medium text-gray-800">{SECTION_TITLES[scope.section]}</span> suggestions again, against
        your CV. It can only change {scope.itemIds.length === 0 ? 'this section' : 'the row below'}, and nothing is saved to your profile
        until you press Insert.
      </p>
      <span data-testid="resuggest-chip" className="inline-block rounded-full bg-purple-50 px-3 py-1 text-xs font-medium text-purple-800">
        {scopeChip(scope)}
      </span>

      {started === null && ai.platforms.length > 0 && (
        <>
          <p className="mt-4 text-xs font-medium text-gray-500">AI for this chat</p>
          <CvAiPicker
            choice={choice}
            settings={{ platform: ai.platform, model: ai.model }}
            platforms={ai.platforms}
            onChange={(platformId, model) => {
              setOverride({ platformId, model })
            }}
            onReset={() => {
              setOverride(null)
            }}
          />
        </>
      )}
      {started !== null && (
        <p className="mt-4 text-xs text-gray-500">
          AI for this chat: <span className="font-medium text-gray-700">{started.name}</span>
        </p>
      )}

      <div role="log" aria-live="polite" data-testid="resuggest-log" className="mt-4 space-y-2">
        {lines.map((l, i) => (
          <p
            key={i}
            data-role={l.role}
            className={`rounded-lg px-3 py-2 text-sm ${
              l.role === 'user'
                ? 'ml-10 bg-gray-100 text-gray-900'
                : l.role === 'ai'
                  ? 'mr-10 bg-purple-50 text-purple-900'
                  : 'bg-red-50 text-red-800'
            }`}
          >
            {l.text}
          </p>
        ))}
        {busy && (
          <p data-testid="resuggest-working" className="flex items-center gap-2 text-sm text-gray-500">
            <span
              aria-hidden="true"
              className="h-3 w-3 animate-spin rounded-full border-2 border-purple-300 border-t-purple-700 motion-reduce:animate-none"
            />
            Working…
          </p>
        )}
      </div>

      <label className="mt-4 block text-xs font-medium text-gray-500">
        {lines.length === 0 ? 'What should change? (optional)' : 'Say what else should change'}
        <textarea
          value={text}
          disabled={busy}
          onChange={(e) => {
            setText(e.target.value)
          }}
          maxLength={MAX_WISH_LENGTH}
          rows={3}
          placeholder="e.g. add the cloud skills from the CV, or shorten the bullets"
          className="mt-1 w-full rounded-md border border-gray-200 px-3 py-2 text-sm text-gray-900 disabled:opacity-60"
        />
      </label>
      <p className="mt-2 text-xs text-gray-500">
        The CV and these suggestions are sent to <span className="font-medium text-gray-700">{aiText || 'the AI platform'}</span>.
        {ai.platforms.length === 0 && ai.loaded && (
          <>
            {' '}
            <button
              type="button"
              onClick={() => {
                window.__cinqoToolBridge.navigate(SETTINGS_PATH)
              }}
              className="text-blue-700 underline"
            >
              Set up an AI in Settings
            </button>
          </>
        )}
      </p>
      <div className="mt-5 flex justify-end gap-2">
        <button
          type="button"
          onClick={onClose}
          disabled={busy}
          className="rounded-md border border-gray-200 px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50"
        >
          Done
        </button>
        <button
          type="button"
          onClick={() => {
            void send()
          }}
          disabled={busy || !choice.platform}
          className="flex items-center gap-1.5 rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white hover:bg-gray-800 disabled:opacity-50"
        >
          <SparkleIcon />
          {busy ? 'Working…' : lines.length === 0 ? 'Re-suggest' : 'Send'}
        </button>
      </div>
    </Modal>
  )
}
