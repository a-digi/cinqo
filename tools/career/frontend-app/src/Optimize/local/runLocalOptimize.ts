import { retryPrompt, buildLocalPrompt, checkAnswer, isLocalCandidate } from './localText'

// The local driver of "Optimize with AI" (plan 258, step 5): for a small local model that cannot follow a prompt with tools (Phi-4-mini). Code reads the texts, asks the model about ONE text
// at a time (no tools, a fresh turn each), checks the answer and hands the good ones over for the person to approve or decline, exactly as the tool does for a remote model. Titles are left
// alone and a text the model cannot improve properly after two tries is left out: nothing is stored for it and nothing is invented.
export interface LocalOptimizeDeps {
  loadTexts: () => Promise<{ kind: string; itemId: string; field: string; label: string; context?: string; text: string }[]>
  // One question as a fresh turn of the run's own conversation, no tools; resolves with the model's answer.
  ask: (prompt: string) => Promise<string>
  store: (s: { kind: string; itemId: string; field: string; text: string; reason: string }) => Promise<void>
  onProgress?: (p: { done: number; total: number }) => void
}

export interface LocalOptimizeResult {
  stored: number
  // Texts the model could not improve properly (a wrong answer twice, or the server refused it).
  skipped: number
  // One plain sentence for the run's last message.
  summary: string
}

// The server holds at most this many suggestions per run.
export const MAX_LOCAL_TEXTS = 100

export async function runLocalOptimize(deps: LocalOptimizeDeps, focus: string): Promise<LocalOptimizeResult> {
  const texts = (await deps.loadTexts()).filter(isLocalCandidate).slice(0, MAX_LOCAL_TEXTS)
  let stored = 0
  let skipped = 0
  deps.onProgress?.({ done: 0, total: texts.length })
  for (const [i, t] of texts.entries()) {
    const prompt = buildLocalPrompt(t, focus)
    let verdict = checkAnswer(t.text, await deps.ask(prompt))
    if (!verdict.ok) verdict = checkAnswer(t.text, await deps.ask(retryPrompt(prompt, verdict.reason)))
    if (verdict.ok) {
      try {
        await deps.store({ kind: t.kind, itemId: t.itemId, field: t.field, text: verdict.text, reason: 'Clearer wording.' })
        stored++
      } catch {
        // The server refused it (a rule the page does not know): this text is left as it is.
        skipped++
      }
    } else {
      skipped++
    }
    deps.onProgress?.({ done: i + 1, total: texts.length })
  }
  const summary =
    stored === 0
      ? 'Done: nothing to improve.'
      : `Done: ${String(stored)} ${stored === 1 ? 'suggestion is' : 'suggestions are'} ready for your review${skipped > 0 ? `; ${String(skipped)} ${skipped === 1 ? 'text was left as it is' : 'texts were left as they are'}` : ''}.`
  return { stored, skipped, summary }
}
