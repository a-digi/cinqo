// Pure display rules for the Home dashboard's widgets: number and delta
// formatting, refresh timing, and chart geometry. No React, so each can be
// script-checked on its own.
import type { ChartData, StatData, WidgetState } from './widgetKinds'

const numberFormat = new Intl.NumberFormat('en', { maximumFractionDigits: 2 })

export function formatStatValue(value: StatData['value']): string {
  return typeof value === 'number' ? numberFormat.format(value) : value
}

export interface DeltaDisplay {
  arrow: string
  text: string
  tone: 'good' | 'bad' | 'neutral'
}

// "good" says whether the direction is desirable (fewer errors is down and
// good); without it the delta is shown neutrally rather than guessing.
export function deltaDisplay(delta: NonNullable<StatData['delta']>): DeltaDisplay {
  const magnitude = numberFormat.format(Math.abs(delta.value))
  const arrow = delta.direction === 'up' ? '▲' : delta.direction === 'down' ? '▼' : '–'
  const sign = delta.direction === 'up' ? '+' : delta.direction === 'down' ? '−' : ''
  const tone = delta.good === undefined ? 'neutral' : delta.good ? 'good' : 'bad'
  return { arrow, text: `${sign}${magnitude}`, tone }
}

export const RETRY_MS = 15_000

// How long to wait before asking for a widget's data again: its own refresh
// interval after a good answer, a shorter retry after a failure so a recovered
// tool comes back soon.
export function nextDelayMs(refreshSeconds: number, outcome: WidgetState | 'request-failed'): number {
  const refresh = refreshSeconds * 1000
  return outcome === 'ok' || outcome === 'empty' ? refresh : Math.min(refresh, RETRY_MS)
}

// What a widget that has nothing to draw says.
export const stateMessages: Record<Exclude<WidgetState, 'ok'>, string> = {
  empty: 'Nothing to show yet.',
  unavailable: 'Not available right now.',
  error: "This widget couldn't load.",
}

export const customMissingMessage = "This widget's display isn't available (its tool may still be loading)."
export const customFailedMessage = "This widget couldn't be displayed."

export const unsupportedMessage = "This widget sent data this version can't show."

// --- chart geometry ---------------------------------------------------------

export interface ChartBox {
  width: number
  height: number
  padX: number
  padTop: number
  padBottom: number
}

export interface ChartGeometry {
  yMin: number
  yMax: number
  baselineY: number
  labels: { x: number; text: string }[]
  lines: { series: number; d: string; area: string; dots: { x: number; y: number }[] }[]
  bars: { series: number; x: number; y: number; w: number; h: number; value: number }[]
}

const MAX_LABELS = 6
// A lone bar should not stretch across the whole card.
const MAX_BAR_WIDTH = 56

function round(n: number): number {
  return Math.round(n * 100) / 100
}

// Lays a chart out in a width x height box. Categories come from the first
// series' x values; the y axis always includes 0, so bars have a true baseline.
export function chartGeometry(data: ChartData, box: ChartBox): ChartGeometry {
  const innerW = box.width - box.padX * 2
  const innerH = box.height - box.padTop - box.padBottom
  const ys = data.series.flatMap((s) => s.points.map((p) => p.y))
  const yMin = Math.min(0, ...ys)
  let yMax = Math.max(0, ...ys)
  if (yMax === yMin) yMax = yMin + 1
  const scaleY = (y: number) => box.padTop + innerH - ((y - yMin) / (yMax - yMin)) * innerH
  const baselineY = round(scaleY(0))

  const categories = data.series.reduce((n, s) => Math.max(n, s.points.length), 0)
  const labelsSource = data.series.find((s) => s.points.length === categories)?.points ?? []
  const step = Math.max(1, Math.ceil(categories / MAX_LABELS))

  const geometry: ChartGeometry = { yMin, yMax, baselineY, labels: [], lines: [], bars: [] }

  if (data.type === 'line') {
    const xAt = (i: number) => (categories <= 1 ? box.padX + innerW / 2 : box.padX + (i * innerW) / (categories - 1))
    labelsSource.forEach((p, i) => {
      if (i % step === 0) geometry.labels.push({ x: round(xAt(i)), text: p.x })
    })
    data.series.forEach((s, series) => {
      if (s.points.length === 0) return
      const dots = s.points.map((p, i) => ({ x: round(xAt(i)), y: round(scaleY(p.y)) }))
      const d = dots.map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x} ${p.y}`).join(' ')
      // The same line closed down to the zero line, for a soft fill under it.
      const area = `${d} L${dots[dots.length - 1].x} ${baselineY} L${dots[0].x} ${baselineY} Z`
      geometry.lines.push({ series, d, area, dots })
    })
    return geometry
  }

  const band = categories === 0 ? 0 : innerW / categories
  const barW = Math.min((band * 0.7) / Math.max(1, data.series.length), MAX_BAR_WIDTH)
  const groupW = barW * data.series.length
  labelsSource.forEach((p, i) => {
    if (i % step === 0) geometry.labels.push({ x: round(box.padX + band * i + band / 2), text: p.x })
  })
  data.series.forEach((s, series) => {
    s.points.forEach((p, i) => {
      const left = box.padX + band * i + (band - groupW) / 2 + barW * series
      const top = Math.min(scaleY(p.y), baselineY)
      geometry.bars.push({
        series,
        x: round(left),
        y: round(top),
        w: round(barW),
        h: round(Math.abs(scaleY(p.y) - baselineY)),
        value: p.y,
      })
    })
  })
  return geometry
}

// Value labels on bars are shown only while there are few enough bars for them to
// stay readable.
export const MAX_LABELLED_BARS = 12

export function showBarValues(bars: ChartGeometry['bars']): boolean {
  return bars.length > 0 && bars.length <= MAX_LABELLED_BARS
}

export function formatBarValue(value: number): string {
  return numberFormat.format(value)
}

// Series colours, readable on white and distinguishable from each other.
export const seriesColors = ['#2563eb', '#16a34a', '#d97706'] as const
