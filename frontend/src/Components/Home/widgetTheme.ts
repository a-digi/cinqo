// How a widget looks: an accent colour, an icon and a decorative shape. Derived
// in the browser (a widget does not carry any of this), so every tool's widgets
// are coloured without the tool doing anything. Pure, so it can be script-checked.
import type { WidgetKind } from './widgetKinds'

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
const accentNames = Object.keys(accents) as AccentName[]

export type WidgetIcon = 'tools' | 'updates' | 'chat' | 'media' | 'briefcase' | 'globe' | 'chart' | 'list' | 'number' | 'puzzle'

// Shapes drawn behind a card's content. Different kinds get different ones, so
// the page is not a wall of identical rectangles.
export type DecorShape = 'circles' | 'diamond' | 'ring'

export interface WidgetTheme {
  accent: Accent
  accentName: AccentName
  icon: WidgetIcon
  shape: DecorShape
}

const knownWidgets: Partial<Record<string, { accent: AccentName; icon: WidgetIcon }>> = {
  'cinqo.tools_overview': { accent: 'teal', icon: 'tools' },
  'cinqo.updates_available': { accent: 'amber', icon: 'updates' },
  'cinqo.recent_conversations': { accent: 'violet', icon: 'chat' },
  'cinqo.media_usage': { accent: 'rose', icon: 'media' },
}

const knownProviders: Partial<Record<string, { accent: AccentName; icon: WidgetIcon }>> = {
  career: { accent: 'blue', icon: 'briefcase' },
  browser: { accent: 'sky', icon: 'globe' },
}

const kindIcons: Record<WidgetKind, WidgetIcon> = {
  stat: 'number',
  list: 'list',
  chart: 'chart',
  custom: 'puzzle',
}

const kindShapes: Record<WidgetKind, DecorShape> = {
  stat: 'circles',
  list: 'diamond',
  chart: 'ring',
  custom: 'circles',
}

// A small stable string hash, so a provider always gets the same colour.
export function hashString(s: string): number {
  let h = 0
  for (const ch of s) h = (h * 31 + (ch.codePointAt(0) ?? 0)) >>> 0
  return h
}

export function widgetTheme(widget: { id: string; provider: string; kind: WidgetKind }): WidgetTheme {
  const known = knownWidgets[widget.id] ?? knownProviders[widget.provider]
  const accentName = known?.accent ?? accentNames[hashString(widget.provider) % accentNames.length]
  // A known provider's own widgets keep its icon, except charts and lists, which
  // are told apart by their kind so two widgets from one tool do not look alike.
  const icon =
    knownWidgets[widget.id]?.icon ??
    (widget.kind === 'chart' || widget.kind === 'list' ? kindIcons[widget.kind] : known?.icon) ??
    kindIcons[widget.kind]
  return { accent: accents[accentName], accentName, icon, shape: kindShapes[widget.kind] }
}

// First letter (or digit) of a title, upper-cased, for an avatar circle.
export function initialOf(text: string): string {
  const first = Array.from(text.trim())[0]
  return first ? first.toUpperCase() : '•'
}
