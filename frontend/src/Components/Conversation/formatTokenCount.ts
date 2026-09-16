// formatTokenCount renders a token count compactly: under 1000 as a
// plain integer, 1000+ as N.NK with a trailing ".0" dropped (1000 ->
// "1K", not "1.0K"; 12300 -> "12.3K"), and — beyond what was
// explicitly asked for, but the same pattern applied consistently
// rather than left to overflow into an unreadable 7-digit number —
// 1,000,000+ as N.NM. See
// plan/ai/conversation/step-34-realtime-token-usage-budget-and-display.md.
export function formatTokenCount(n: number): string {
  if (n < 1000) return String(n)
  if (n < 1_000_000) return trimTrailingZero(n / 1000) + 'K'
  return trimTrailingZero(n / 1_000_000) + 'M'
}

function trimTrailingZero(n: number): string {
  return n.toFixed(1).replace(/\.0$/, '')
}
