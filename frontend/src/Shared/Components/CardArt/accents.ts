// The accent colours shared by the dashboard cards and the tool cards: a gradient pair, a pale tint and a
// readable dark shade each. Pure (no React), so it can be script-checked.

export interface Accent {
  // Gradient ends for the icon badge, bars and the big number.
  from: string
  to: string
  // A pale tint for backgrounds and chips, and a readable dark shade for text.
  soft: string
  text: string
}

export const accents = {
  blue: { from: '#60a5fa', to: '#2563eb', soft: '#eff6ff', text: '#1d4ed8' },
  green: { from: '#4ade80', to: '#16a34a', soft: '#f0fdf4', text: '#15803d' },
  amber: { from: '#fbbf24', to: '#d97706', soft: '#fffbeb', text: '#b45309' },
  rose: { from: '#fb7185', to: '#e11d48', soft: '#fff1f2', text: '#be123c' },
  violet: { from: '#a78bfa', to: '#7c3aed', soft: '#f5f3ff', text: '#6d28d9' },
  teal: { from: '#2dd4bf', to: '#0d9488', soft: '#f0fdfa', text: '#0f766e' },
  sky: { from: '#38bdf8', to: '#0284c7', soft: '#f0f9ff', text: '#0369a1' },
  orange: { from: '#fb923c', to: '#ea580c', soft: '#fff7ed', text: '#c2410c' },
} as const satisfies Record<string, Accent>

export type AccentName = keyof typeof accents
export const accentNames = Object.keys(accents) as AccentName[]

// A small stable string hash, so a provider always gets the same colour.
export function hashString(s: string): number {
  let h = 0
  for (const ch of s) h = (h * 31 + (ch.codePointAt(0) ?? 0)) >>> 0
  return h
}
