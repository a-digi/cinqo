import { SETTINGS_PATH } from '../../paths'

// Shown on the Import from CV page when the AI chosen (in the picker of the page, which starts from Settings) is a model that was measured not to manage this task (plan 207): the 1.5B
// local model reads the CV and then answers every step with a made-up "done". It is a warning, not a block: the person may still try.
export function UnsuitableModelNotice({ choice }: { choice: string }) {
  return (
    <div
      role="status"
      data-testid="unsuitable-model-notice"
      className="mb-5 rounded-2xl border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-900"
    >
      <p className="font-semibold">This model cannot do the CV import</p>
      <p className="mt-1">
        {choice} was measured not to manage it: it reads the CV, then answers the next steps without storing anything. Choose another model
        above (Qwen2.5-Coder-3B or a cloud model), or change the Career setting, or the import will most likely fail.
      </p>
      <button
        type="button"
        onClick={() => {
          window.__cinqoToolBridge.navigate(SETTINGS_PATH)
        }}
        className="mt-2 rounded-full border border-amber-400 bg-white px-3 py-1 text-xs font-medium text-amber-900 hover:bg-amber-100"
      >
        Open AI settings
      </button>
    </div>
  )
}
