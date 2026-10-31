// Pure rules for showing a photo's details (the profile view's photo modal). No React, so they can be
// script-checked.

// What core's media list says about one file (a subset: the fields the modal shows).
export interface MediaMeta {
  id: string
  original_filename: string
  content_type: string
  size_bytes: number
  created_at: string
  width?: number
  height?: number
  title?: string
}

// "512 B", "12.3 KB", "1.5 MB": the unit that keeps the number under 1024; '' for a size that is not a
// number or negative.
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return ''
  if (bytes < 1024) return `${Math.round(bytes)} B`
  const units = ['KB', 'MB', 'GB']
  let value = bytes / 1024
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit++
  }
  return `${value.toFixed(value < 10 ? 1 : 0)} ${units[unit]}`
}

// "800 × 600"; '' when either side is missing or not positive.
export function formatDimensions(width: number | undefined, height: number | undefined): string {
  return width !== undefined && height !== undefined && width > 0 && height > 0 ? `${width} × ${height}` : ''
}

export interface MetaRow {
  label: string
  value: string
}

// The rows of the modal, in order; a row with nothing to say is left out. `addedAt` is when the photo was
// added to this profile, `usedBy` the names of every profile that has it, `format` turns a timestamp into
// text ('' when it cannot be read).
export function photoMetaRows(args: {
  isDefault: boolean
  addedAt: string
  meta: MediaMeta | null
  usedBy: string[]
  format: (iso: string) => string
}): MetaRow[] {
  const { isDefault, addedAt, meta, usedBy, format } = args
  const rows: MetaRow[] = [
    { label: 'Default photo', value: isDefault ? 'Yes' : 'No' },
    { label: 'Added to this profile', value: format(addedAt) },
  ]
  if (meta) {
    rows.push(
      { label: 'File name', value: meta.original_filename },
      { label: 'Type', value: meta.content_type },
      { label: 'Size', value: formatBytes(meta.size_bytes) },
      { label: 'Dimensions', value: formatDimensions(meta.width, meta.height) },
      { label: 'Uploaded', value: format(meta.created_at) },
    )
  }
  if (usedBy.length > 0) rows.push({ label: 'Used by', value: usedBy.join(', ') })
  return rows.filter((r) => r.value !== '')
}
