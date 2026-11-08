// askPiece.ts — asks a model one small question and checks what it answers (plan 209, step 3).
//
// A piece is one thing the import needs (the name, the persona, the skills, ...): the question to put to the model (its own small piece of
// the CV, no tools), the check of its answer, and what code can cut out of the CV itself when the model's answer is not usable. runPiece
// asks, reads the JSON, checks it, asks once more with the exact fault, and then falls back to the code's own reading. It says where each
// value came from, so a weak result can be explained. A piece with neither a good answer nor a fallback gives null: it is left out and
// nothing is invented.

import { parseModelJson } from './modelJson'

export type Verdict<T> = { ok: true; value: T } | { ok: false; reason: string }

export interface Piece<T> {
  // What the model is told: only this piece's text, and the JSON shape wanted.
  prompt: string
  // The model's parsed JSON, checked: the value, or the reason it is not usable (told to the model on the second ask).
  validate: (parsed: unknown) => Verdict<T>
  // What code can read from the CV itself, or null.
  fallback: () => T | null
}

export interface PieceOutcome<T> {
  value: T | null
  source: 'model' | 'code' | 'none'
  // Why the model's answer was not used ('' when it was).
  note: string
}

// The second ask: the same question, and what was wrong with the first answer.
export function retryPrompt(prompt: string, reason: string): string {
  return `${prompt}\n\nYour earlier answer could not be used: ${reason}\nAnswer again with only the JSON.`
}

export async function runPiece<T>(piece: Piece<T>, ask: (prompt: string) => Promise<string>, retries = 1): Promise<PieceOutcome<T>> {
  let prompt = piece.prompt
  let reason = ''
  for (let attempt = 0; attempt <= retries; attempt++) {
    let answer: string
    try {
      answer = await ask(prompt)
    } catch (err) {
      reason = `the model gave no answer (${err instanceof Error ? err.message : String(err)})`
      break
    }
    const parsed = parseModelJson(answer)
    if (parsed === undefined) {
      reason = 'it was not JSON'
    } else {
      const verdict = piece.validate(parsed)
      if (verdict.ok) return { value: verdict.value, source: 'model', note: '' }
      reason = verdict.reason
    }
    prompt = retryPrompt(piece.prompt, reason)
  }
  const value = piece.fallback()
  return value === null ? { value: null, source: 'none', note: reason } : { value, source: 'code', note: reason }
}
