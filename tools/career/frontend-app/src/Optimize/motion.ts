// motion.ts — the timing of the animations of "Optimize with AI" (plan 258, step 7). The animations themselves are in index.css (the `opt-*` classes); the review keeps a decided card in the
// page until its animation has ended, so it needs the same numbers: keep the two in step. With reduced motion nothing waits: everything is shown at once.

// A decided card first flashes or slides (LEAVE_BASE_MS), then its row folds away (FOLD_MS); a card that leaves together with others starts STAGGER_MS after the one before it.
export const LEAVE_BASE_MS = 180
export const FOLD_MS = 420
export const STAGGER_MS = 60
// A little more than the last animation, so the card is never cut off.
export const LEAVE_MARGIN_MS = 40
// "All suggestions decided" is shown this long before the review closes.
export const DONE_PAUSE_MS = 600
// The "N changes applied" chip stays this long, the last FADE_MS of it fading out.
export const CHIP_SHOW_MS = 4000
export const CHIP_FADE_MS = 400

export function prefersReducedMotion(): boolean {
  return (
    typeof window !== 'undefined' &&
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches
  )
}

// How long the cards that leave together stay in the page: until the last one has folded away.
export function leaveTotalMs(count: number, reduced = prefersReducedMotion()): number {
  if (reduced || count <= 0) return 0
  return LEAVE_BASE_MS + FOLD_MS + STAGGER_MS * (count - 1) + LEAVE_MARGIN_MS
}

export function donePauseMs(reduced = prefersReducedMotion()): number {
  return reduced ? 0 : DONE_PAUSE_MS
}

// The words said (to a screen reader) about the cards that were just decided.
export function announcement(kind: 'approved' | 'declined', labels: string[]): string {
  const word = kind === 'approved' ? 'Approved' : 'Declined'
  const first = labels[0] ?? ''
  if (labels.length <= 1) return `${word}: ${first}`
  return `${word}: ${first} and ${String(labels.length - 1)} more`
}

export function appliedText(count: number): string {
  return `${String(count)} ${count === 1 ? 'change' : 'changes'} applied`
}
