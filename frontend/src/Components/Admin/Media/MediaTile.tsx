import { IconButton } from '../../../Shared/Components/IconButton/IconButton'
import { PencilIcon, TrashIcon } from '../../../Shared/Components/IconButton/icons'
import { Pill } from '../../../Shared/Components/Pill/Pill'
import type { MediaFile } from '../../../api/media'
import { MediaThumbnail } from './MediaThumbnail'
import { displayName, formatSize, mediaKind } from './mediaFilters'

const kindLabel = { image: 'Image', pdf: 'PDF', other: 'File' } as const

export interface MediaTileProps {
  file: MediaFile
  selected: boolean
  busy: boolean
  onToggle: () => void
  onOpen: () => void
  onDelete: () => void
}

// One media file as a tile: thumbnail on top (image, PDF first page or a file
// glyph), name and facts below. The checkbox and the edit/delete actions fade
// in on hover or focus, and stay visible while the tile is selected.
export function MediaTile({ file, selected, busy, onToggle, onOpen, onDelete }: MediaTileProps) {
  const name = displayName(file)
  const controls = selected
    ? 'opacity-100'
    : 'opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 [@media(hover:none)]:opacity-100'
  return (
    <div className="group min-w-0">
      <div
        className={`relative aspect-[4/3] overflow-hidden rounded-xl bg-gray-100 shadow-sm transition duration-200 group-hover:-translate-y-0.5 group-hover:shadow-md ${
          selected ? 'ring-2 ring-blue-600 ring-offset-2' : ''
        }`}
      >
        <button type="button" onClick={onOpen} aria-label={`Open ${name}`} className="block h-full w-full cursor-pointer">
          <MediaThumbnail file={file} />
        </button>
        <label
          className={`absolute left-2 top-2 flex h-6 w-6 cursor-pointer items-center justify-center rounded bg-white shadow transition-opacity ${controls}`}
        >
          <input
            type="checkbox"
            checked={selected}
            onChange={onToggle}
            aria-label={`Select ${name}`}
            className="h-4 w-4 cursor-pointer accent-blue-600"
          />
        </label>
        <div className={`absolute right-2 top-2 flex gap-1.5 transition-opacity ${controls}`}>
          <IconButton floating icon={<PencilIcon />} label="Edit" onClick={onOpen} />
          <IconButton floating variant="danger" icon={<TrashIcon />} label="Delete" onClick={onDelete} disabled={busy} />
        </div>
      </div>
      <p className="mt-2 line-clamp-2 break-words text-sm text-gray-900" title={name}>
        {name}
      </p>
      <div className="mt-1 flex flex-wrap items-center gap-1.5 text-xs text-gray-500">
        <Pill outline>{file.toolSlug}</Pill>
        <span>
          {kindLabel[mediaKind(file)]} · {formatSize(file.sizeBytes)}
        </span>
      </div>
    </div>
  )
}
