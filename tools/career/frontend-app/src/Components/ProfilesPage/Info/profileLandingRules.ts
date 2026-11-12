// Pure rules of the landing layout of the Profiles infographic (plan 267). No React, so they can be script-checked.

// How many tiles go in each row for `count` caption tiles: pairs, and a last one on its own across the page when the number is odd.
export function captionRows(count: number): number[] {
  const rows: number[] = []
  for (let left = Math.max(0, Math.floor(count)); left > 0; left -= 2) rows.push(left >= 2 ? 2 : 1)
  return rows
}

// The variant of the caption tile at `index` (0 first): the first `tall` are tall, the others half; a tile alone in its row spans the page (wide).
export function captionVariant(index: number, count: number, tall: number): 'tall' | 'half' | 'wide' {
  const rows = captionRows(count)
  const lastAlone = rows[rows.length - 1] === 1 && index === count - 1
  if (lastAlone) return 'wide'
  return index < tall ? 'tall' : 'half'
}

// "Where to click: New profile, Import CV", or '' when there is nothing to click.
export function clickLine(click: string[]): string {
  return click.length === 0 ? '' : `Where to click: ${click.join(', ')}`
}
