// A copy of the Career tool's Components/JobsPage/jobsTheme.ts (colours): a tool's build is self-contained, so the Browser tool
// keeps its own small copy of the shared look (see tools/career/frontend-app/src/Components/JobsPage/jobsTheme.ts).
// Same palette, same blue page accent, so the two tools read as one product.
export interface Accent {
  from: string
  to: string
  soft: string
  text: string
}

export const ACCENTS: Accent[] = [
  { from: '#60a5fa', to: '#2563eb', soft: '#eff6ff', text: '#1d4ed8' }, // blue
  { from: '#4ade80', to: '#16a34a', soft: '#f0fdf4', text: '#15803d' }, // green
  { from: '#fbbf24', to: '#d97706', soft: '#fffbeb', text: '#b45309' }, // amber
  { from: '#fb7185', to: '#e11d48', soft: '#fff1f2', text: '#be123c' }, // rose
  { from: '#a78bfa', to: '#7c3aed', soft: '#f5f3ff', text: '#6d28d9' }, // violet
  { from: '#2dd4bf', to: '#0d9488', soft: '#f0fdfa', text: '#0f766e' }, // teal
  { from: '#38bdf8', to: '#0284c7', soft: '#f0f9ff', text: '#0369a1' }, // sky
  { from: '#fb923c', to: '#ea580c', soft: '#fff7ed', text: '#c2410c' }, // orange
]

// The page's own accent (the Career blue).
export const PAGE_ACCENT: Accent = ACCENTS[0]

// A small stable string hash.
export function hashString(s: string): number {
  let h = 0
  for (const ch of s) h = (h * 31 + (ch.codePointAt(0) ?? 0)) >>> 0
  return h
}

export function accentFor(key: string): Accent {
  return ACCENTS[hashString(key.trim().toLowerCase()) % ACCENTS.length]
}

// First letter (or digit) for an avatar circle; a bullet when there is none.
export function initialOf(text: string): string {
  const first = Array.from(text.trim())[0]
  return first ? first.toUpperCase() : '•'
}

export function gradient(a: Accent): string {
  return `linear-gradient(135deg, ${a.from}, ${a.to})`
}
