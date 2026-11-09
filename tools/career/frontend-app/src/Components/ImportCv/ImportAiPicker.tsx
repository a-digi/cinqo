import { defaultModelFor } from '../../Cinqo/Platform/aiChoice'
import { LocalModelNote } from '../../Cinqo/Platform/LocalModelNote'
import { modelLabel, modelOptions, showsLocalModelNote } from '../../Cinqo/Platform/modelLabels'
import type { Platform } from '../../Cinqo/Platform/platformRepository'
import { SETTINGS_PATH } from '../../paths'
import { cardClassName, cardStyle } from '../../Shared/cardStyle'
import { Dropdown } from '../Dropdown/Dropdown'
import type { ImportAiChoice, ImportAiSettings } from './importAiChoice'

// The AI of this import (plan 212): the platform and the model, starting from the Career AI settings, which the person may change for this import only.
// Nothing is saved from here and nothing is remembered: the Career settings stay the one place that saves, and opening the page starts from them again.
export function ImportAiPicker({
  choice,
  settings,
  platforms,
  onChange,
  onReset,
}: {
  choice: ImportAiChoice
  settings: ImportAiSettings
  platforms: Platform[]
  onChange: (platformId: string, model: string) => void
  onReset: () => void
}) {
  const platform = choice.platform
  const settingsText = settings.platform
    ? `${settings.platform.name}${settings.model ? ` · ${modelLabel(settings.platform, settings.model)}` : ''}`
    : 'nothing usable yet'
  return (
    <section data-testid="import-ai-picker" className={`career-enter mb-5 ${cardClassName}`} style={cardStyle}>
      <h2 className="text-sm font-semibold text-gray-900">AI for this import</h2>
      <div className="mt-3 flex flex-wrap items-end gap-4 [&>div]:min-w-0">
        <div className="w-60 [&>div]:block [&>div>button]:w-full">
          <Dropdown
            label="Platform"
            options={platforms.map((p) => ({ value: p.id, label: p.name }))}
            value={platform?.id ?? null}
            onChange={(id) => {
              const next = platforms.find((p) => p.id === id)
              if (next) onChange(next.id, defaultModelFor(next))
            }}
          />
        </div>
        {platform && platform.models.length > 0 && (
          <div className="w-72 [&>div]:block [&>div>button]:w-full">
            <Dropdown
              label="Model"
              options={modelOptions(platform)}
              value={choice.model}
              onChange={(m) => {
                onChange(platform.id, m)
              }}
            />
          </div>
        )}
      </div>
      {showsLocalModelNote(platform) && <LocalModelNote platform={platform} />}
      <p className="mt-3 text-xs text-gray-500">
        Career AI settings: <span className="font-medium text-gray-700">{settingsText}</span>.{' '}
        <button
          type="button"
          onClick={() => {
            window.__cinqoToolBridge.navigate(SETTINGS_PATH)
          }}
          className="underline hover:text-gray-900"
        >
          Open AI settings
        </button>
        {choice.source === 'override' && (
          <>
            {' · '}
            <button type="button" onClick={onReset} className="underline hover:text-gray-900">
              Use the Career setting
            </button>
          </>
        )}
      </p>
      {choice.source === 'settings-after-override' && (
        <p role="status" className="mt-2 text-xs text-amber-800">
          The platform you picked can no longer be used (its key is gone, or the Local AI is off): the Career AI settings are used.
        </p>
      )}
    </section>
  )
}
