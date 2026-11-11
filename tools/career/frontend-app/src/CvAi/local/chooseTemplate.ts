import type { CvTemplate } from '../../api'
import type { Piece } from '../../Cinqo/Conversation/toolFree/askPiece'

// The one thing a small local model decides when it creates a CV (plan 253): which design. Everything else is done by code. The model is shown a short list (not all 110
// designs) and answers with one id; an answer that is not on the list is refused and asked for again once, and when it is still unusable code takes the fallback.

// The design code takes when the model gave no usable answer: the plainest ATS-friendly one for a CV without a photo, a plain one-column design built to show the photo for a
// CV with one (plan 256).
export const FALLBACK_TEMPLATE_ID = 'ats-clean'
export const FALLBACK_WITH_PHOTO_ID = 'modern-mono'

// The category of the designs meant to have no photo (plan 255).
const ATS_CATEGORY = 'ATS-Friendly'

// What the model is offered: designs built to show a photo (the CV has one), the ATS designs (it has none), or, when the note asks for ATS although the CV has a photo, the ATS
// designs first and the rest after them.
export type CandidateMode = 'photo' | 'photo-ats' | 'no-photo'

const MAX_CANDIDATES = 8
const MAX_MATCHED = 5

function words(text: string): Set<string> {
  return new Set(
    text
      .toLowerCase()
      .split(/[^\p{L}\p{N}]+/u)
      .filter((w) => w.length >= 3),
  )
}

// A design matches when one of its "best for" tags shares a word with what the person is (headline, target role) or wrote about the CV's purpose.
function matchScore(template: CvTemplate, about: Set<string>): number {
  let score = 0
  for (const tag of template.bestFor) {
    for (const w of words(tag)) if (about.has(w)) score++
  }
  return score
}

const isAts = (t: CvTemplate) => t.category === ATS_CATEGORY

// The designs the model may choose from, at most MAX_CANDIDATES. The pool depends on the mode: 'photo' the designs that are not ATS, 'no-photo' the ATS designs, 'photo-ats' all of
// them with the ATS designs ahead. In the pool, first the best matches of the "best for" tags with the person (best match first, at most MAX_MATCHED), then the rest in the order
// of the list (the picker order: the two-column photo designs first). The fallback design of the mode is always in the list. The order is stable.
export function candidateTemplates(templates: CvTemplate[], about: string, mode: CandidateMode = 'photo'): CvTemplate[] {
  const ats = templates.filter(isAts)
  const rest = templates.filter((t) => !isAts(t))
  const pool = mode === 'photo' ? rest : mode === 'no-photo' ? ats : [...ats, ...rest]
  const source = pool.length > 0 ? pool : templates
  const aboutWords = words(about)
  const matched = source
    .map((t, i) => ({ t, i, score: matchScore(t, aboutWords) }))
    .filter((x) => x.score > 0)
    .sort((a, b) => b.score - a.score || a.i - b.i)
    .slice(0, MAX_MATCHED)
    .map((x) => x.t)
  const out: CvTemplate[] = []
  const add = (t: CvTemplate | undefined) => {
    if (t && !out.some((o) => o.id === t.id) && out.length < MAX_CANDIDATES) out.push(t)
  }
  matched.forEach(add)
  source.forEach(add)
  const fallback = source.find((t) => t.id === fallbackIdFor(mode))
  if (fallback && !out.some((o) => o.id === fallback.id)) out[out.length - 1] = fallback
  return out
}

function fallbackIdFor(mode: CandidateMode): string {
  return mode === 'photo' ? FALLBACK_WITH_PHOTO_ID : FALLBACK_TEMPLATE_ID
}

// The design code takes when the model's answer is not usable: the fallback of the mode when it is among the candidates, else the first candidate.
export function fallbackTemplateId(candidates: CvTemplate[], mode: CandidateMode = 'photo'): string | null {
  if (candidates.length === 0) return null
  const wanted = fallbackIdFor(mode)
  return candidates.some((c) => c.id === wanted) ? wanted : candidates[0].id
}

export function buildTemplatePrompt(
  about: { name: string; headline: string; focus: string; photo: boolean },
  candidates: CvTemplate[],
): string {
  const lines = candidates.map((c) => `- ${c.id}: ${c.name} (${c.category}; best for ${c.bestFor.join(', ') || 'any profession'})`)
  const wish = about.focus.trim()
  return [
    `Choose the CV design that suits this person best.`,
    `Person: ${about.name}${about.headline.trim() ? `, ${about.headline.trim()}` : ''}.`,
    wish ? `What the CV is for: ${JSON.stringify(wish)}` : 'What the CV is for: a good general CV.',
    about.photo ? "The CV will show the person's photo." : 'The CV will have no photo.',
    '',
    'Designs:',
    ...lines,
    '',
    `Answer with this JSON and nothing else: {"templateId": "<one of the ids above>"}`,
  ].join('\n')
}

export function templatePiece(
  about: { name: string; headline: string; focus: string; photo: boolean },
  candidates: CvTemplate[],
  mode: CandidateMode = 'photo',
): Piece<string> {
  const ids = candidates.map((c) => c.id)
  return {
    prompt: buildTemplatePrompt(about, candidates),
    validate: (parsed) => {
      const id = typeof parsed === 'object' && parsed !== null ? (parsed as { templateId?: unknown }).templateId : undefined
      if (typeof id !== 'string') return { ok: false, reason: 'templateId is missing: answer {"templateId": "<id>"}' }
      const wanted = id.trim()
      if (!ids.includes(wanted))
        return { ok: false, reason: `"${wanted}" is not one of the designs; use exactly one of: ${ids.join(', ')}` }
      return { ok: true, value: wanted }
    },
    fallback: () => fallbackTemplateId(candidates, mode),
  }
}
