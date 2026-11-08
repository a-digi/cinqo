// importContext.ts — whether the AI platform can hold what a CV import needs at once, decided before the import starts.
//
// A platform can state how much a conversation on it holds (the local AI: the context size of its model). The import is a long
// conversation: the instructions (about 2,900 tokens with the tools), the text of the CV (about 3,300 tokens for a two-page one,
// more for a long one), and then everything the AI hands over in pieces, all in the same context. Measured on the real import with
// Phi-4-mini (2026-11-03): the first two requests alone were 6,232 tokens. 16,384 is the least that works for an ordinary CV.
// With less, the import fails after the file was read, with an error about the conversation being too long. A platform that
// states no context (the cloud platforms) is never blocked here.
export const IMPORT_MIN_CONTEXT = 16384

// The message for a platform whose context is too small, or null when the import may start (the context is large enough, or the
// platform does not state one).
export function importContextProblem(contextTokens: number | undefined | null): string | null {
  if (typeof contextTokens !== 'number' || !Number.isFinite(contextTokens) || contextTokens <= 0) return null
  if (contextTokens >= IMPORT_MIN_CONTEXT) return null
  return `The AI reads ${contextTokens.toLocaleString('en-US')} tokens at once, and an import needs at least ${IMPORT_MIN_CONTEXT.toLocaleString('en-US')}: the instructions, the text of the CV and what the AI hands over all share it. Raise the context size under AI, Local AI, then try again.`
}
