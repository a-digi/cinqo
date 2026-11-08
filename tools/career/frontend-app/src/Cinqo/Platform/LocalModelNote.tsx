import { LOCAL_AI_PATH, LOCAL_MODEL_NOTE } from './modelLabels'

// The note under a local platform's model picker (plan 206): one model runs at a time, and where to get more of them. The link opens the
// Cinqo Local AI page (downloads, switch on/off, default model); choosing among the downloaded models needs no visit there.
export function LocalModelNote() {
  return (
    <p className="text-xs text-gray-500" data-testid="local-model-note">
      {LOCAL_MODEL_NOTE}{' '}
      <button
        type="button"
        onClick={() => {
          window.__cinqoToolBridge.navigate(LOCAL_AI_PATH)
        }}
        className="font-medium text-gray-700 underline underline-offset-2 hover:no-underline focus:outline-none focus-visible:ring-2 focus-visible:ring-gray-400"
      >
        Get more models
      </button>
    </p>
  )
}
