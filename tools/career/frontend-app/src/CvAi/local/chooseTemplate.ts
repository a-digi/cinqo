import type { CvTemplate } from '../../api'
import type { Piece } from '../../Cinqo/Conversation/toolFree/askPiece'

// The one thing a small local model decides when it creates a CV (plan 253): which design. Everything else is done by code. The model is shown a short list (not all 110
// designs) and answers with one id; an answer that is not on the list is refused and asked for again once, and when it is still unusable code takes the fallback.

// The design code takes when the model gave no usable answer: the plainest ATS-friendly one.
export const FALLBACK_TEMPLATE_ID = 'ats-clean'

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

// The designs the model may choose from, at most MAX_CANDIDATES: first the ones whose "best for" matches the person (best match first, at most MAX_MATCHED), then the
// ATS-friendly designs in their own order, and always the fallback design. The order is stable, so the same CV gets the same list.
export function candidateTemplates(templates: CvTemplate[], about: string): CvTemplate[] {
  const aboutWords = words(about)
  const scored = templates
    .map((t, i) => ({ t, i, score: matchScore(t, aboutWords) }))
    .filter((x) => x.score > 0)
    .sort((a, b) => b.score - a.score || a.i - b.i)
    .slice(0, MAX_MATCHED)
    .map((x) => x.t)
  const out: CvTemplate[] = []
  const add = (t: CvTemplate | undefined) => {
    if (t && !out.some((o) => o.id === t.id) && out.length < MAX_CANDIDATES) out.push(t)
  }
  scored.forEach(add)
  templates.filter((t) => t.category === 'ATS-Friendly').forEach(add)
  templates.forEach(add)
  const fallback = templates.find((t) => t.id === FALLBACK_TEMPLATE_ID)
  if (fallback && !out.some((o) => o.id === fallback.id)) out[out.length - 1] = fallback
  return out
}

// The design code takes when the model's answer is not usable: the fallback design when it is among the candidates, else the first candidate.
export function fallbackTemplateId(candidates: CvTemplate[]): string | null {
  if (candidates.length === 0) return null
  return candidates.some((c) => c.id === FALLBACK_TEMPLATE_ID) ? FALLBACK_TEMPLATE_ID : candidates[0].id
}

export function buildTemplatePrompt(about: { name: string; headline: string; focus: string }, candidates: CvTemplate[]): string {
  const lines = candidates.map((c) => `- ${c.id}: ${c.name} (${c.category}; best for ${c.bestFor.join(', ') || 'any profession'})`)
  const wish = about.focus.trim()
  return [
    `Choose the CV design that suits this person best.`,
    `Person: ${about.name}${about.headline.trim() ? `, ${about.headline.trim()}` : ''}.`,
    wish ? `What the CV is for: ${JSON.stringify(wish)}` : 'What the CV is for: a good general CV.',
    '',
    'Designs:',
    ...lines,
    '',
    `Answer with this JSON and nothing else: {"templateId": "<one of the ids above>"}`,
  ].join('\n')
}

export function templatePiece(about: { name: string; headline: string; focus: string }, candidates: CvTemplate[]): Piece<string> {
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
    fallback: () => fallbackTemplateId(candidates),
  }
}
