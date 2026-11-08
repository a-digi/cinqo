import { accents, accentNames, hashString, type Accent, type AccentName } from '../../../Shared/Components/CardArt/accents'
import type { WidgetIcon } from '../../../Shared/Components/CardArt/icons'
import type { DecorShape } from '../../../Shared/Components/CardArt/shapes'

// How a tool's card head looks (plan/ai/tools/tool-card-heads.md): an accent, an icon and a shape, worked out in
// the browser from what the tool list already carries, so no tool has to declare anything. Pure (no React), so it
// can be script-checked.

export type ToolKind = 'frontend_only' | 'backend_only' | 'frontend_and_backend'

export interface ToolTheme {
  accentName: AccentName
  accent: Accent
  icon: WidgetIcon
  shape: DecorShape
}

// The tools that ship with Cinqo keep the identity they already have on the dashboard.
const known: Partial<Record<string, { accent: AccentName; icon: WidgetIcon }>> = {
  career: { accent: 'blue', icon: 'briefcase' },
  browser: { accent: 'sky', icon: 'globe' },
  pdf_tools: { accent: 'orange', icon: 'document' },
}

const kindIcons: Record<ToolKind, WidgetIcon> = {
  frontend_only: 'puzzle',
  backend_only: 'server',
  frontend_and_backend: 'layers',
}

const shapes: DecorShape[] = ['circles', 'diamond', 'ring']

// `key` is the tool's slug (installed tools) or its id in a repository (the same word); `kind` is optional (a
// repository's listing does not carry it) and only picks the icon of a tool this file does not know.
export function themeForTool(key: string, kind?: ToolKind): ToolTheme {
  const id = key.trim().toLowerCase()
  const match = known[id]
  const accentName = match?.accent ?? accentNames[hashString(id) % accentNames.length]
  return {
    accentName,
    accent: accents[accentName],
    icon: match?.icon ?? (kind ? kindIcons[kind] : 'puzzle'),
    // its own hash (not the accent's), so a colour does not always come with the same shape
    shape: shapes[hashString(`${id}/shape`) % shapes.length],
  }
}

// ---- readable white text on the head's gradient ----

function parseHex(hex: string): [number, number, number] {
  const h = hex.replace('#', '')
  return [parseInt(h.slice(0, 2), 16), parseInt(h.slice(2, 4), 16), parseInt(h.slice(4, 6), 16)]
}

function toHex([r, g, b]: [number, number, number]): string {
  return `#${[r, g, b]
    .map((v) =>
      Math.round(Math.min(255, Math.max(0, v)))
        .toString(16)
        .padStart(2, '0'),
    )
    .join('')}`
}

// The WCAG relative luminance of a colour, 0 (black) to 1 (white).
function luminance(hex: string): number {
  const channel = (v: number) => {
    const s = v / 255
    return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4)
  }
  const [r, g, b] = parseHex(hex)
  return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b)
}

// The WCAG contrast ratio of white text on a background colour (1 to 21).
export function contrastWithWhite(background: string): number {
  return 1.05 / (luminance(background) + 0.05)
}

function mix(a: string, b: string, t: number): string {
  const [ar, ag, ab] = parseHex(a)
  const [br, bg, bb] = parseHex(b)
  return toHex([ar + (br - ar) * t, ag + (bg - ag) * t, ab + (bb - ab) * t])
}

// The colour darkened, in small steps toward black, only as far as white text on it reaches `min` (4.5, the
// WCAG minimum for body text, by default). A colour that already reads is returned as it is.
export function readableShade(hex: string, min = 4.5): string {
  let shade = hex
  for (let i = 0; i < 40 && contrastWithWhite(shade) < min; i++) shade = mix(shade, '#000000', 0.04)
  return shade
}

// The two ends of a head's gradient: the accent's midpoint and its deep end, each darkened just enough for white
// text to be readable on it, so the name is readable wherever it sits on the gradient.
export function headGradient(accent: Accent): { from: string; to: string } {
  return { from: readableShade(mix(accent.from, accent.to, 0.5)), to: readableShade(accent.to) }
}
