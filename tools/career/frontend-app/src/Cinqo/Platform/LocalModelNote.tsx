import { useState } from 'react'
import { LOCAL_AI_PATH, LOCAL_MODEL_NOTE } from './modelLabels'
import {
  needsQwenCoderDownload,
  opensLocalAgents,
  outcomeText,
  suggestErrorText,
  suggestQwenCoder,
  type SuggestOutcome,
} from './suggestLocalModel'

type Ask = { kind: 'idle' } | { kind: 'busy' } | { kind: 'done'; outcome: SuggestOutcome } | { kind: 'error'; text: string }

// The note under a local platform's model picker (plan 206): one model runs at a time, and where to get more of them. The link opens the
// Cinqo Local AI page (downloads, switch on/off, default model); choosing among the downloaded models needs no visit there.
// plan 220: when Qwen2.5-Coder-3B, the best local model for crawl instructions, is not among the models, a second line offers to ask Cinqo for it:
// Career only says which model it wants, Cinqo asks the person on its Local Agents page and does the download. The picker stays as it is (this can
// sit inside a dialog), so the answer is shown here and the page is opened only when the person chooses to.
export function LocalModelNote({ platform }: { platform: { models: string[] } }) {
  const [ask, setAsk] = useState<Ask>({ kind: 'idle' })
  const offer = needsQwenCoderDownload(platform)

  function askCinqo() {
    setAsk({ kind: 'busy' })
    suggestQwenCoder()
      .then((outcome) => {
        setAsk({ kind: 'done', outcome })
      })
      .catch((err: unknown) => {
        setAsk({ kind: 'error', text: suggestErrorText(err) })
      })
  }

  return (
    <div className="space-y-1" data-testid="local-model-note">
      <p className="text-xs text-gray-500">
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
      {offer && (
        <p className="text-xs text-gray-500" data-testid="ask-cinqo-qwen-coder">
          {ask.kind === 'done' ? (
            <>
              {outcomeText(ask.outcome)}{' '}
              {opensLocalAgents(ask.outcome) && (
                <button
                  type="button"
                  onClick={() => {
                    window.__cinqoToolBridge.navigate(LOCAL_AI_PATH)
                  }}
                  className="font-medium text-gray-700 underline underline-offset-2 hover:no-underline focus:outline-none focus-visible:ring-2 focus-visible:ring-gray-400"
                >
                  Open the Local Agents page
                </button>
              )}
            </>
          ) : (
            <>
              Qwen2.5-Coder-3B, the best local model for crawl instructions, is not downloaded.{' '}
              <button
                type="button"
                disabled={ask.kind === 'busy'}
                onClick={askCinqo}
                className="font-medium text-gray-700 underline underline-offset-2 hover:no-underline focus:outline-none focus-visible:ring-2 focus-visible:ring-gray-400 disabled:opacity-50"
              >
                {ask.kind === 'busy' ? 'Asking Cinqo…' : 'Ask Cinqo to download it'}
              </button>
            </>
          )}
          {ask.kind === 'error' && (
            <span role="alert" className="ml-1 text-red-700">
              {ask.text}
            </span>
          )}
        </p>
      )}
    </div>
  )
}
