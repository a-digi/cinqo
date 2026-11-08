// The Home dashboard's widget contract, mirrored from the backend
// (api/src/dashboard/widget.go and kinds.go). The engine already validates every
// payload before it is sent; these guards repeat the check in the browser so a
// bug or a version skew shows "unsupported data" instead of crashing a renderer.
// Keep the limits and rules in step with the Go file. See
// plan/ai/dashboard/home-dashboard.md.

export const SCHEMA_VERSION = 1

export type WidgetKind = 'stat' | 'list' | 'chart' | 'custom'
export type WidgetState = 'ok' | 'empty' | 'unavailable' | 'error'
export type Variant = 'gray' | 'green' | 'amber' | 'red'

export interface StatData {
  label: string
  value: string | number
  unit?: string
  delta?: { value: number; direction: 'up' | 'down' | 'flat'; good?: boolean }
  caption?: string
  link?: string
}

export interface ListItem {
  title: string
  subtitle?: string
  meta?: string
  badge?: { text: string; variant: Variant }
  link?: string
}

export interface ListData {
  items: ListItem[]
}

export interface ChartData {
  type: 'bar' | 'line'
  series: { name: string; points: { x: string; y: number }[] }[]
}

// A tool-drawn widget's data: any JSON object, which only that tool's own renderer
// understands. Core checks only that it is an object.
export type CustomData = Record<string, unknown>

export interface WidgetDataByKind {
  stat: StatData
  list: ListData
  chart: ChartData
  custom: CustomData
}

export const limits = {
  listItems: 10,
  chartSeries: 3,
  chartPoints: 60,
  label: 80,
  title: 120,
  text: 200,
  short: 40,
  badge: 30,
  link: 300,
} as const

type Obj = Record<string, unknown>

function isObj(v: unknown): v is Obj {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}

function isFinite_(v: unknown): v is number {
  return typeof v === 'number' && Number.isFinite(v)
}

// Characters, not UTF-16 units, to match the backend's rune count.
function chars(s: string): number {
  return Array.from(s).length
}

function requiredText(v: unknown, max: number): v is string {
  return typeof v === 'string' && v.trim() !== '' && chars(v.trim()) <= max
}

function optionalText(v: unknown, max: number): boolean {
  return v === undefined || (typeof v === 'string' && chars(v.trim()) <= max)
}

// A link is usable only if it is an internal app path: one leading "/", not
// protocol-relative, no backslash or control character. Otherwise it is ignored
// (the card simply is not clickable), same as the backend, which drops it.
export function safeLink(link: unknown): string | undefined {
  if (typeof link !== 'string') return undefined
  const l = link.trim()
  if (l === '' || l.length > limits.link || !l.startsWith('/')) return undefined
  if (l.length > 1 && (l[1] === '/' || l[1] === '\\')) return undefined
  for (const ch of l) {
    const code = ch.codePointAt(0) ?? 0
    if (ch === '\\' || code < 0x20 || code === 0x7f) return undefined
  }
  return l
}

function isStat(v: unknown): v is StatData {
  if (!isObj(v)) return false
  if (!requiredText(v.label, limits.label)) return false
  const value = v.value
  if (!(isFinite_(value) || requiredText(value, limits.label))) return false
  if (!optionalText(v.unit, limits.short) || !optionalText(v.caption, limits.text)) return false
  if (v.delta !== undefined) {
    const d = v.delta
    if (!isObj(d) || !isFinite_(d.value)) return false
    if (d.direction !== 'up' && d.direction !== 'down' && d.direction !== 'flat') return false
    if (d.good !== undefined && typeof d.good !== 'boolean') return false
  }
  return true
}

const variants = ['gray', 'green', 'amber', 'red']

function isListItem(v: unknown): v is ListItem {
  if (!isObj(v)) return false
  if (!requiredText(v.title, limits.title)) return false
  if (!optionalText(v.subtitle, limits.text) || !optionalText(v.meta, limits.short)) return false
  if (v.badge !== undefined) {
    const b = v.badge
    if (!isObj(b) || !requiredText(b.text, limits.badge)) return false
    if (typeof b.variant !== 'string' || !variants.includes(b.variant)) return false
  }
  return true
}

function isList(v: unknown): v is ListData {
  return isObj(v) && Array.isArray(v.items) && v.items.length <= limits.listItems && v.items.every(isListItem)
}

function isChart(v: unknown): v is ChartData {
  if (!isObj(v) || (v.type !== 'bar' && v.type !== 'line')) return false
  if (!Array.isArray(v.series) || v.series.length > limits.chartSeries) return false
  return v.series.every(
    (s: unknown) =>
      isObj(s) &&
      requiredText(s.name, limits.short) &&
      Array.isArray(s.points) &&
      s.points.length <= limits.chartPoints &&
      s.points.every((p: unknown) => isObj(p) && requiredText(p.x, limits.short) && isFinite_(p.y)),
  )
}

// Returns the typed payload for `kind`, or null when it does not match the
// schema (or the schema version is not the one this build understands).
export function parseWidgetData<K extends WidgetKind>(kind: K, schemaVersion: number, data: unknown): WidgetDataByKind[K] | null {
  if (schemaVersion !== SCHEMA_VERSION) return null
  const ok =
    kind === 'stat'
      ? isStat(data)
      : kind === 'list'
        ? isList(data)
        : kind === 'chart'
          ? isChart(data)
          : kind === 'custom'
            ? isObj(data)
            : false
  return ok ? (data as WidgetDataByKind[K]) : null
}

export function isWidgetKind(v: unknown): v is WidgetKind {
  return v === 'stat' || v === 'list' || v === 'chart' || v === 'custom'
}
