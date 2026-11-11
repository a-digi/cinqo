import { defaultModelFor } from '../Cinqo/Platform/aiChoice'
import type { ResolvedAi, AiSettingsChoice } from '../Cinqo/Platform/aiOverride'
import { LocalModelNote } from '../Cinqo/Platform/LocalModelNote'
import { modelLabel, modelOptions, showsLocalModelNote } from '../Cinqo/Platform/modelLabels'
import type { Platform } from '../Cinqo/Platform/platformRepository'
import { Dropdown } from '../Components/Dropdown/Dropdown'
import { SETTINGS_PATH } from '../paths'

// The AI of one "Create a CV with AI" run (plan 252): the platform and the model, starting from the Career AI settings, which the person may change for this run only.
// Nothing is saved from here and nothing is remembered: the Career settings stay the one place that saves, and opening the dialog starts from them again.
export function CvAiPicker({
  choice,
  settings,
  platforms,
  onChange,
  onReset,
}: {
  choice: ResolvedAi
  settings: AiSettingsChoice
  platforms: Platform[]
  onChange: (platformId: string, model: string) => void
  onReset: () => void
}) {
  const platform = choice.platform
  const settingsText = settings.platform
    ? `${settings.platform.name}${settings.model ? ` · ${modelLabel(settings.platform, settings.model)}` : ''}`
    : 'nothing usable yet'
  return (
    <div data-testid="cv-ai-picker" className="mt-3">
      <div className="flex flex-wrap items-end gap-4 [&>div]:min-w-0">
        <div className="w-52 [&>div]:block [&>div>button]:w-full">
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
          <div className="w-64 [&>div]:block [&>div>button]:w-full">
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
      <p className="mt-2 text-xs text-gray-500">
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
    </div>
  )
}
