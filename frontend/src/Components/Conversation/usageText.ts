// What the provider reported for a turn besides the prompt and completion counts, as short text (plan/ai/conversation/step-178,
// step 3): "8K cached · 600 reasoning · $0.0123". Pure (no React), so it can be script-checked. Every part is left out when the provider
// reported nothing for it, so a provider that reports no extras shows exactly what was shown before; nothing is ever guessed.
import type { UsageExtras } from '../../api/conversations'
import { formatTokenCount } from './formatTokenCount'

function count(n: number | undefined): number {
  return typeof n === 'number' && Number.isFinite(n) && n > 0 ? Math.floor(n) : 0
}

function cost(n: number | undefined): number | null {
  return typeof n === 'number' && Number.isFinite(n) && n >= 0 ? n : null
}

// A cost in US dollars: "free" for 0, "<$0.0001" below that, four decimals under a dollar ("$0.0123", small amounts stay readable),
// two decimals from a dollar up with thousands separators ("$1,234.50").
export function formatCostUsd(dollars: number): string {
  if (dollars === 0) return 'free'
  if (dollars < 0.0001) return '<$0.0001'
  if (dollars < 1) return `$${dollars.toFixed(4)}`
  return `$${dollars.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`
}

// The parts that go after the prompt and completion counts, each led by " · ", or '' when the provider reported none:
// " · 8K cached · 300 cache write · 600 reasoning · $0.0123".
export function usageExtrasText(extras: UsageExtras | undefined): string {
  if (!extras) return ''
  const parts: string[] = []
  const read = count(extras.cacheReadTokens)
  if (read > 0) parts.push(`${formatTokenCount(read)} cached`)
  const write = count(extras.cacheWriteTokens)
  if (write > 0) parts.push(`${formatTokenCount(write)} cache write`)
  const reasoning = count(extras.reasoningTokens)
  if (reasoning > 0) parts.push(`${formatTokenCount(reasoning)} reasoning`)
  const dollars = cost(extras.costUsd)
  if (dollars !== null) parts.push(formatCostUsd(dollars))
  return parts.map((p) => ` · ${p}`).join('')
}

// Only the extras of a response (a poll answer carries many other fields): what is kept for a turn that is being watched.
export function pickUsageExtras(source: UsageExtras): UsageExtras {
  const out: UsageExtras = {}
  if (source.cacheReadTokens !== undefined) out.cacheReadTokens = source.cacheReadTokens
  if (source.cacheWriteTokens !== undefined) out.cacheWriteTokens = source.cacheWriteTokens
  if (source.reasoningTokens !== undefined) out.reasoningTokens = source.reasoningTokens
  if (source.costUsd !== undefined) out.costUsd = source.costUsd
  return out
}
