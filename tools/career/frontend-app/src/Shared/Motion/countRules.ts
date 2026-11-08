// Pure rules for the count-up: how a displayed figure ("42", "75%", "1,234", "0 of 6", "2.5") is split into
// a prefix, the number to animate and a suffix, and put back together. No React, so they can be script-checked.
export interface ParsedCount {
  prefix: string
  value: number
  // Digits after the decimal point in the original, kept while counting.
  decimals: number
  // The original used a thousands comma ("1,234"), so the counting figure gets one too.
  grouped: boolean
  suffix: string
}

// The first number in the text, with what is around it; null when there is none (a dash, a date).
export function parseCount(text: string): ParsedCount | null {
  const m = /^([^\d-]*)(-?\d[\d,]*(?:\.\d+)?)(.*)$/.exec(text)
  if (!m) return null
  const raw = m[2]
  const value = Number(raw.replace(/,/g, ''))
  if (!Number.isFinite(value)) return null
  // A date or a range (29.10.2026, 2016-03, 3/4) is not a figure to count.
  if (/^[./:-]\d/.test(m[3])) return null
  const dot = raw.indexOf('.')
  return { prefix: m[1], value, decimals: dot === -1 ? 0 : raw.length - dot - 1, grouped: raw.includes(','), suffix: m[3] }
}

// The text at some point of the count: current is between 0 and parsed.value.
export function formatCount(parsed: ParsedCount, current: number): string {
  const fixed = current.toFixed(parsed.decimals)
  const shown = parsed.grouped
    ? Number(fixed).toLocaleString('en-US', { minimumFractionDigits: parsed.decimals, maximumFractionDigits: parsed.decimals })
    : fixed
  return `${parsed.prefix}${shown}${parsed.suffix}`
}

// A smooth start and a gentle landing: progress t (0 to 1) to the share of the way there.
export function easeOut(t: number): number {
  const c = Math.min(1, Math.max(0, t))
  return 1 - Math.pow(1 - c, 3)
}
