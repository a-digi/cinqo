// Pure rules for the tool bridge's pdfThumbnail: which ids it accepts and how a render is cached.
// No React, so they can be script-checked.

// Media file ids are UUIDs (api/src/media). The bridge takes an id and nothing else, so a tool can
// never point the preview at some other URL.
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

export function isMediaId(value: unknown): value is string {
  return typeof value === 'string' && UUID.test(value)
}

// One render per file and version: a tool passes the document's own updatedAt as the version, so
// a file that is replaced under the same id is rendered again instead of shown stale.
export function thumbnailCacheKey(id: string, version?: string): string {
  return `${id.toLowerCase()}|${version ?? ''}`
}

// True for every cache key that belongs to this file, whatever its version.
export function isKeyForId(key: string, id: string): boolean {
  return key.startsWith(`${id.toLowerCase()}|`)
}
