// localText.ts — the small questions of the local driver of "Optimize with AI" (plan 258, step 5) and the checks of the answers. A small local model (Phi-4-mini) cannot follow a prompt with
// tools, so code does the work: it asks the model ONE text at a time, with no tools, and reads the plain answer. The model's answer is never trusted: it is cleaned (the wrappers a small
// model puts around a text) and checked, and a wrong one is asked for once more with the exact fault; an answer that is still wrong is left out and nothing is stored for that text.
// Nothing here imports the remote driver.
import { LANGUAGE_NAMES, detectLanguage } from '../language'

// The texts the local driver works on: a title is left alone (a small model changes what a job title means) and so is a text too short to say anything.
export const MIN_LOCAL_TEXT = 15
export const MAX_TEXT = 4000

export function isLocalCandidate(t: { field: string; text: string }): boolean {
  return t.field !== 'title' && t.text.trim().length >= MIN_LOCAL_TEXT
}

const FIELD_WORDS: Record<string, string> = { summary: 'summary', description: 'description' }

// The language rule, said in the question: always, and by name when the text can be placed (a small model asked in English about a German text answers in English unless it is told).
function languageLines(text: string): string[] {
  const lang = detectLanguage(text)
  const name = lang ? LANGUAGE_NAMES[lang] : ''
  return [
    lang
      ? `This text is ${name}. Write the improved text in ${name}, the language of the text, even though these instructions are in English. Never translate.`
      : 'Write the improved text in the SAME language as the text, even though these instructions are in English. Never translate.',
  ]
}

export function buildLocalPrompt(t: { field: string; label: string; context?: string; text: string }, focus: string): string {
  const wanted = focus.trim()
  return [
    `Improve the wording of this ${FIELD_WORDS[t.field] ?? 'text'}${t.label ? ` (${t.label}${t.context ? `, ${t.context}` : ''})` : ''}.`,
    ...languageLines(t.text),
    'Rules: keep the language of the text. Keep every fact, number, percentage, name, date and technology. Add nothing the text does not say. Fix typos and grammar and make clumsy or wordy sentences read well.',
    'Keep the line breaks. Every line that starts with "- " must stay a line that starts with "- ": never drop a bullet point and never merge several into one.',
    ...(wanted ? [`The person asked for this (a wish about the wording only): ${JSON.stringify(wanted)}`] : []),
    'Reply with ONLY the improved text. No introduction, no explanation, no quotes around it, no markdown.',
    '',
    'Text:',
    '<<<',
    t.text,
    '>>>',
  ].join('\n')
}

// Takes the wrappers off a model's answer: a code fence, the <<< >>> marks of the question, an "Improved text:" line in front, quotes around the whole text (only when the original had none).
export function cleanAnswer(answer: string, before: string): string {
  let a = answer.trim()
  const fence = /^```[a-z]*\n([\s\S]*?)\n?```$/i.exec(a)
  if (fence) a = fence[1].trim()
  a = a
    .replace(/^<<<\s*/, '')
    .replace(/\s*>>>$/, '')
    .trim()
  a = a
    .replace(
      /^(?:here(?:'s| is) the improved (?:text|version|summary|description)|improved (?:text|version|summary|description)|improved)\s*:\s*/i,
      '',
    )
    .trim()
  const quoted = /^(["“„'])([\s\S]*)(["”“'])$/.exec(a)
  if (quoted && !/^["“„']/.test(before.trim())) a = quoted[2].trim()
  return a
}

export function bulletLines(text: string): number {
  return text.split('\n').filter((l) => l.trim().startsWith('- ')).length
}

export type Verdict = { ok: true; text: string } | { ok: false; reason: string }

// The checks of an answer: the same rules the server applies (not empty, changed, not too long, no bullet point lost) and two that catch a small model's usual faults (a text cut short, a text grown into an essay).
export function checkAnswer(before: string, answer: string): Verdict {
  const text = cleanAnswer(answer, before)
  if (text === '') return { ok: false, reason: 'the answer was empty' }
  if (text === before.trim()) return { ok: false, reason: 'the answer is the same as the text' }
  if (text.length > MAX_TEXT) return { ok: false, reason: `the answer is longer than ${String(MAX_TEXT)} characters` }
  if (bulletLines(text) < bulletLines(before))
    return { ok: false, reason: 'a bullet point was dropped or merged: keep every line that starts with "- "' }
  const want = detectLanguage(before)
  const got = detectLanguage(text)
  if (want && got && want !== got) {
    return {
      ok: false,
      reason: `the answer is ${LANGUAGE_NAMES[got]} but the text is ${LANGUAGE_NAMES[want]}: write it in ${LANGUAGE_NAMES[want]}, the language of the text, and never translate`,
    }
  }
  if (text.length < before.trim().length * 0.5) return { ok: false, reason: 'the answer is much shorter than the text: keep every fact' }
  if (text.length > before.trim().length * 2.5) return { ok: false, reason: 'the answer is much longer than the text: add nothing' }
  return { ok: true, text }
}

export function retryPrompt(prompt: string, reason: string): string {
  return `${prompt}\n\nYour earlier answer could not be used: ${reason}.\nAnswer again with ONLY the improved text.`
}
