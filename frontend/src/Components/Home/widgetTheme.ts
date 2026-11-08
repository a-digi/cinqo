// How a widget looks: an accent colour, an icon and a decorative shape. Derived
// in the browser (a widget does not carry any of this), so every tool's widgets
// are coloured without the tool doing anything. Pure, so it can be script-checked.
import { accents, accentNames, hashString, type Accent, type AccentName } from '../../Shared/Components/CardArt/accents'
import type { WidgetIcon } from '../../Shared/Components/CardArt/icons'
import type { DecorShape } from '../../Shared/Components/CardArt/shapes'
import type { WidgetKind } from './widgetKinds'

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
