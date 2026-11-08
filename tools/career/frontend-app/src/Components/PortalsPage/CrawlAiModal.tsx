import { useEffect, useState } from 'react'
import { defaultModelFor } from '../../Cinqo/Platform/aiChoice'
import type { AiOverride } from '../../Cinqo/Platform/crawlAiChoice'
import { LocalModelNote } from '../../Cinqo/Platform/LocalModelNote'
import { modelOptions, showsLocalModelNote } from '../../Cinqo/Platform/modelLabels'
import type { Platform } from '../../Cinqo/Platform/platformRepository'
import { InfoBox } from '../../Shared/InfoBox/InfoBox'
import { Modal } from '../../Shared/Modal/Modal'
import { Dropdown } from '../Dropdown/Dropdown'
import { aiChip } from './aiOverrideDisplay'

// Sets or removes the AI platform and model of one portal or one link (step 102). The
// same modal serves both: the caller says which it is, what is saved now, and what
// would be used without an override (the next level down). Saving and removing are the
// caller's (they call the API and refresh the list); this only collects the choice.
export function CrawlAiModal({
  open,
  title,
  subject,
  current,
  platforms,
  fallbackText,
  suggested,
  onClose,
  onSave,
  onRemove,
}: {
  open: boolean
  title: string
  subject: 'portal' | 'link'
  current: AiOverride
  platforms: Platform[]
  // What is used when this level has no choice, e.g. "OpenRouter · model (the portal's choice)".
  fallbackText: string
  // A choice to start from instead of what is saved (the recommendation pill, plan 213); the person still has to Save it.
  suggested?: { platformId: string; model: string } | null // model '' = only the platform is suggested
  onClose: () => void
  onSave: (platformId: string, model: string) => Promise<void>
  onRemove: () => Promise<void>
}) {
  const savedPlatformId = (current.platformId ?? '').trim()
  const savedModel = (current.model ?? '').trim()
  const stale = savedPlatformId !== '' && !platforms.some((p) => p.id === savedPlatformId)
  const hasOverride = savedPlatformId !== ''

  // The suggested choice, when its platform can be used. Plain strings, so the effect below only restarts when they really change
  // (the platforms array is a new one on every render).
  const wantedId = suggested && platforms.some((p) => p.id === suggested.platformId) ? suggested.platformId : ''
  const wantedModel = wantedId !== '' ? (suggested?.model ?? '') : ''

  const [platformId, setPlatformId] = useState('')
  const [model, setModel] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  // Each time the modal opens, start from what is saved (a stale platform starts empty, to be picked again).
  useEffect(() => {
    if (!open) return
    setPlatformId(wantedId !== '' ? wantedId : stale ? '' : savedPlatformId)
    setModel(wantedId !== '' ? wantedModel : stale ? '' : savedModel)
    setBusy(false)
    setError('')
  }, [open, stale, savedPlatformId, savedModel, wantedId, wantedModel])

  const selected = platforms.find((p) => p.id === platformId) ?? null
  const unchanged = !stale && platformId === savedPlatformId && model === savedModel
  // A platform that lists models needs one picked (a suggestion may name only the platform).
  const needsModel = selected !== null && selected.models.length > 0 && model === ''

  function pickPlatform(id: string) {
    setPlatformId(id)
    const next = platforms.find((p) => p.id === id)
    setModel(next ? defaultModelFor(next) : '')
    setError('')
  }

  function run(action: () => Promise<void>) {
    setBusy(true)
    setError('')
    action()
      .then(onClose)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
        setBusy(false)
      })
  }

  return (
    <Modal open={open} title={title} onClose={onClose}>
      <p className="mb-3 text-xs text-gray-500">
        {subject === 'portal'
          ? 'The links of this platform that have no choice of their own use this one.'
          : 'Crawling this link, and preparing its crawl instructions, uses this choice.'}
      </p>

      {platforms.length === 0 ? (
        <InfoBox
          variant="warning"
          label="No AI platform"
          message="No AI platform can be used yet. Switch on the Local AI, or add an API key for a platform, in the AI menu of Cinqo."
        />
      ) : (
        <div className="space-y-3">
          {stale && (
            <InfoBox
              variant="warning"
              label="Platform not available"
              message="The platform saved here can no longer be used (its key is gone, or the Local AI is off), so it is skipped. Pick a new one, or remove the choice."
            />
          )}
          <div className="[&>div]:block [&>div>button]:w-full">
            <Dropdown
              label="Platform"
              options={platforms.map((p) => ({ value: p.id, label: p.name }))}
              value={platformId || null}
              onChange={pickPlatform}
            />
          </div>
          {selected && selected.models.length > 0 && (
            <div className="[&>div]:block [&>div>button]:w-full">
              <Dropdown
                label="Model"
                options={modelOptions(selected)}
                value={model || null}
                onChange={(m) => {
                  setModel(m)
                  setError('')
                }}
              />
            </div>
          )}
          {showsLocalModelNote(selected) && <LocalModelNote />}
        </div>
      )}

      <p className="mt-4 text-xs text-gray-500">
        Saved now: <span className="font-medium text-gray-700">{aiChip(current, platforms).text.replace('AI: ', '')}</span>
        <br />
        {hasOverride ? 'Without it this uses: ' : 'Nothing set, this uses: '}
        <span className="font-medium text-gray-700">{fallbackText}</span>
      </p>

      {error && (
        <p role="alert" className="mt-3 text-sm text-red-700">
          {error}
        </p>
      )}

      <div className="mt-4 flex items-center gap-3">
        <button
          type="button"
          disabled={busy || platformId === '' || unchanged || needsModel}
          onClick={() => {
            run(() => onSave(platformId, model))
          }}
          className="rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800 disabled:opacity-50"
        >
          {busy ? 'Saving…' : 'Save'}
        </button>
        {hasOverride && (
          <button
            type="button"
            disabled={busy}
            onClick={() => {
              run(onRemove)
            }}
            className="text-sm text-red-700 underline hover:text-red-900 disabled:opacity-50"
          >
            Remove choice
          </button>
        )}
        <button
          type="button"
          onClick={onClose}
          className="ml-auto rounded-md border border-gray-200 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50"
        >
          Cancel
        </button>
      </div>
    </Modal>
  )
}
