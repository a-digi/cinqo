// Image types the server's image endpoint can decode (it resizes and
// re-encodes them as JPEG). Everything else is stored untouched.
const processableImageTypes = ['image/jpeg', 'image/png', 'image/gif']

export function usesImageEndpoint(file: Pick<File, 'type'>): boolean {
  return processableImageTypes.includes(file.type)
}

// "my-photo.final.png" -> "my-photo.final": the default title for a dropped file.
export function defaultTitle(filename: string): string {
  const dot = filename.lastIndexOf('.')
  const base = dot > 0 ? filename.slice(0, dot) : filename
  return base.trim() || filename
}

export interface PendingUpload {
  key: string
  file: File
  title: string
  status: 'pending' | 'uploading' | 'error'
  error: string
}

let nextKey = 0

export function toPendingUploads(files: File[]): PendingUpload[] {
  return files.map((file) => ({
    key: `upload-${++nextKey}`,
    file,
    title: defaultTitle(file.name),
    status: 'pending',
    error: '',
  }))
}

export function canUpload(items: PendingUpload[]): boolean {
  return items.length > 0 && items.every((i) => i.title.trim() !== '' && i.status !== 'uploading')
}

// Cropping is offered only for a lone image: with several files in the list a
// crop per file would turn a quick drop into a chore.
export function canCrop(items: PendingUpload[]): boolean {
  return items.length === 1 && usesImageEndpoint(items[0].file) && items[0].status !== 'uploading'
}

// "photo.png" -> "photo.jpg": a crop is re-encoded as JPEG.
export function croppedFilename(filename: string): string {
  const dot = filename.lastIndexOf('.')
  return `${dot > 0 ? filename.slice(0, dot) : filename}.jpg`
}
