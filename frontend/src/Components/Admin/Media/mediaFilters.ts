import type { MediaFile } from '../../../api/media'

export type MediaKind = 'image' | 'pdf' | 'other'
export type MediaTypeFilter = 'any' | MediaKind
export type MediaSort = 'newest' | 'oldest' | 'name' | 'largest'

export interface MediaFilters {
  query: string
  type: MediaTypeFilter
  sort: MediaSort
}

export const defaultMediaFilters: MediaFilters = { query: '', type: 'any', sort: 'newest' }

export const typeOptions: { value: MediaTypeFilter; label: string }[] = [
  { value: 'any', label: 'Any type' },
  { value: 'image', label: 'Images' },
  { value: 'pdf', label: 'PDFs' },
  { value: 'other', label: 'Other' },
]

export const sortOptions: { value: MediaSort; label: string }[] = [
  { value: 'newest', label: 'Newest first' },
  { value: 'oldest', label: 'Oldest first' },
  { value: 'name', label: 'Name A–Z' },
  { value: 'largest', label: 'Largest first' },
]

export function mediaKind(file: Pick<MediaFile, 'contentType'>): MediaKind {
  if (file.contentType.startsWith('image/')) return 'image'
  if (file.contentType === 'application/pdf') return 'pdf'
  return 'other'
}

export function displayName(file: Pick<MediaFile, 'title' | 'originalFilename'>): string {
  return file.title || file.originalFilename
}

export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

// Filters by name (title or original filename) and type, then sorts. Pure:
// the page's selection and tile rendering sit on top of its result.
export function applyMediaFilters(files: MediaFile[], filters: MediaFilters): MediaFile[] {
  const query = filters.query.trim().toLowerCase()
  const matches = files.filter((file) => {
    if (filters.type !== 'any' && mediaKind(file) !== filters.type) return false
    if (query === '') return true
    return displayName(file).toLowerCase().includes(query) || file.originalFilename.toLowerCase().includes(query)
  })
  const byDate = (a: MediaFile, b: MediaFile) => a.createdAt.localeCompare(b.createdAt)
  const sorted = [...matches]
  switch (filters.sort) {
    case 'oldest':
      return sorted.sort(byDate)
    case 'name':
      return sorted.sort((a, b) => displayName(a).localeCompare(displayName(b), undefined, { sensitivity: 'base' }))
    case 'largest':
      return sorted.sort((a, b) => b.sizeBytes - a.sizeBytes || byDate(b, a))
    default:
      return sorted.sort((a, b) => byDate(b, a))
  }
}

export const MAX_PDF_PREVIEW_BYTES = 25 * 1024 * 1024

export function canPreviewPdf(file: Pick<MediaFile, 'sizeBytes'>): boolean {
  return file.sizeBytes <= MAX_PDF_PREVIEW_BYTES
}

export function bulkDeleteMessage(files: MediaFile[]): string {
  const shown = files.slice(0, 5).map((f) => `"${displayName(f)}"`)
  const more = files.length > shown.length ? ` and ${files.length - shown.length} more` : ''
  return `Delete ${files.length} media file${files.length === 1 ? '' : 's'} (${shown.join(', ')}${more})? This removes the stored files and cannot be undone.`
}
