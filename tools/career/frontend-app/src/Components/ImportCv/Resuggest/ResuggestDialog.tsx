import { useEffect, useRef, useState } from 'react'
import { resolveAiOverride, type AiOverride } from '../../../Cinqo/Platform/aiOverride'
import { usesCodeDriver } from '../../../Cinqo/Platform/modelLabels'
import { useAiChoice } from '../../../Cinqo/Platform/useAiChoice'
import { CvAiPicker } from '../../../CvAi/CvAiPicker'
import { SETTINGS_PATH } from '../../../paths'
import { Modal } from '../../../Shared/Modal/Modal'
import { SparkleIcon } from '../../../Shared/Icons/icons'
import { startLocalResuggest, type LocalResuggestChat } from './local/startLocalResuggest'
import { startResuggestChat, type ResuggestChat } from './startResuggestChat'
import {
  LOCAL_MODEL_NOTE,
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
  // The AI picker is a secondary thing: closed until the person asks to change it.
  const [pickerOpen, setPickerOpen] = useState(false)
  // The chat exists from the first send on; its AI is then fixed.
  const [started, setStarted] = useState<{ name: string; local: boolean } | null>(null)
  const chat = useRef<ResuggestChat | null>(null)
  // The comment is what the dialog is for: the cursor starts in it.
  const box = useRef<HTMLTextAreaElement>(null)
  useEffect(() => {
    box.current?.focus()
  }, [])
  const firstSent = useRef(false)
  // A small local model cannot call tools: the page asks it small questions and applies the answers itself (plan 268, step 4). `localChat` is that chat; `progress` says where it is.
  const localChat = useRef<LocalResuggestChat | null>(null)
  const [progress, setProgress] = useState('')

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
    const local = localChat.current !== null || (chat.current === null && usesCodeDriver(platform, choice.model))
    const wish = wishOf(text)
    add({ role: 'user', text: text.trim() === '' ? DEFAULT_WISH : text.trim() })
    setText('')
    setBusy(true)
    try {
      const name = `${platform.name}${choice.model ? ` · ${choice.model}` : ''}`
      if (local) {
        if (localChat.current === null) {
          localChat.current = await startLocalResuggest(run, scope, { platform, model: choice.model, source: ai.source })
          setStarted({ name, local: true })
        }
        const result = await localChat.current.run(wish, setProgress)
        add({ role: 'ai', text: result.summary })
      } else {
        if (chat.current === null) {
          chat.current = await startResuggestChat(run, scope, { platform, model: choice.model, source: ai.source })
          setStarted({ name, local: false })
        }
        const wasFirst = !firstSent.current
        const reply = wasFirst ? await chat.current.first(wish) : await chat.current.next(wish)
        firstSent.current = true
        add({ role: 'ai', text: reply.content.trim() !== '' ? reply.content.trim() : 'The AI made no reply.' })
      }
    } catch (err) {
      add({ role: 'error', text: turnFailure(err) })
    }
    try {
      await onTurnEnd()
    } catch {
      // the page looks again at its next poll
    }
    setProgress('')
    setBusy(false)
  }

  const isLocal = started !== null ? started.local : usesCodeDriver(choice.platform, choice.model)
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
            {progress !== '' ? progress : 'Working…'}
          </p>
        )}
      </div>

      <label className="mt-5 block text-sm font-semibold text-gray-900">
        {lines.length === 0 ? 'What should change?' : 'Say what else should change'}
        <textarea
          value={text}
          disabled={busy}
          ref={box}
          onChange={(e) => {
            setText(e.target.value)
          }}
          maxLength={MAX_WISH_LENGTH}
          rows={5}
          placeholder="e.g. add the cloud skills from the CV, or shorten the bullets. Leave it empty and the AI looks at this again and improves it."
          className="mt-2 w-full rounded-lg border border-gray-300 px-3.5 py-3 text-sm font-normal text-gray-900 shadow-sm focus:border-purple-400 focus:outline-none focus:ring-2 focus:ring-purple-100 disabled:opacity-60"
        />
      </label>

      <div data-testid="resuggest-ai-line" className="mt-3 text-xs text-gray-500">
        <span>
          AI: <span className="font-medium text-gray-700">{aiText || 'none set up'}</span>
        </span>
        {started === null && ai.platforms.length > 0 && (
          <>
            {' · '}
            <button
              type="button"
              aria-expanded={pickerOpen}
              onClick={() => {
                setPickerOpen((v) => !v)
              }}
              className="underline hover:text-gray-900"
            >
              {pickerOpen ? 'Hide' : 'Change'}
            </button>
          </>
        )}
        {ai.platforms.length === 0 && ai.loaded && (
          <>
            {' · '}
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
        <span className="block text-gray-400">Your CV and these suggestions are sent to it.</span>
        {isLocal && (
          <span data-testid="resuggest-local-note" className="mt-1 block text-gray-500">
            {LOCAL_MODEL_NOTE}
          </span>
        )}
        {started === null && pickerOpen && ai.platforms.length > 0 && (
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
        )}
      </div>
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
