import { EyeIcon, TrashIcon } from '../../Shared/Icons/icons'

const base =
  'flex items-center justify-center bg-white text-gray-700 shadow transition-colors hover:bg-blue-50 hover:text-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500'

// The view icon and the remove icon of a photo, the same buttons wherever a photo shows them (the profile and
// persona photo grids, the profile list's picture): their labels, tooltips and look are written once here.
// `compact` is the smaller size for a small picture. The page decides when they show (on hover or focus) and
// what they do; onRemove left out draws only the view icon.
export function PhotoActionButtons({
  isDefault,
  compact = false,
  onView,
  onRemove,
}: {
  isDefault: boolean
  compact?: boolean
  onView: () => void
  onRemove?: () => void
}) {
  const size = compact ? 'h-5 w-5 rounded-md [&_svg]:h-3 [&_svg]:w-3' : 'h-7 w-7 rounded-lg'
  return (
    <>
      <button
        type="button"
        aria-label={isDefault ? 'View the default photo' : 'View this photo'}
        title="View"
        onClick={onView}
        className={`${base} ${size}`}
      >
        <EyeIcon />
      </button>
      {onRemove && (
        <button
          type="button"
          aria-label={isDefault ? 'Remove the default photo from this profile' : 'Remove this photo from this profile'}
          title="Remove from this profile (it stays in the photo library)"
          onClick={onRemove}
          className={`${base} ${size} hover:bg-red-50 hover:text-red-700`}
        >
          <TrashIcon />
        </button>
      )}
    </>
  )
}
