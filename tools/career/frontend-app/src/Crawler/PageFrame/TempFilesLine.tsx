import { useEffect, useState } from 'react'

import { TrashIcon } from '../../Shared/Icons/icons'
import { deleteAllSnapshots, fetchSnapshotStats, type SnapshotStats } from '../linkCrawl'
import { buttonClass } from '../instructionStyles'
import { formatBytes } from '../pageView'

// The temporary page files the Browser tool holds (plan 247/248): how many, how big, and a button that deletes them. It reads the count when the picker opens and
// again whenever a page copy was made or went away (the copy's id). Shared by the pickers that show a page copy.
export function TempFilesLine({ open, snapshotId }: { open: boolean; snapshotId: string }) {
  const [files, setFiles] = useState<SnapshotStats | null>(null)
  const [note, setNote] = useState('')

  function refresh() {
    fetchSnapshotStats()
      .then(setFiles)
      .catch(() => {
        setFiles(null)
      })
  }
  useEffect(() => {
    if (open) refresh()
  }, [open, snapshotId])

  function deleteFiles() {
    setNote('')
    deleteAllSnapshots()
      .then((n) => {
        setNote(
          n === 0
            ? 'There was nothing to delete.'
            : `Deleted ${String(n)} temporary page file${n === 1 ? '' : 's'}. The page in view stays until you close this.`,
        )
        refresh()
      })
      .catch((err: unknown) => {
        setNote(err instanceof Error ? err.message : 'Something went wrong.')
      })
  }

  if (!files) return null
  return (
    <div className="flex flex-wrap items-center gap-2 text-xs text-gray-500" data-testid="temp-files">
      <span>
        Temporary page files: {files.count}
        {files.count > 0 ? ` (${formatBytes(files.bytes)})` : ''}
      </span>
      <button
        type="button"
        className={`${buttonClass} inline-flex items-center gap-1.5`}
        disabled={files.count === 0}
        onClick={deleteFiles}
      >
        <TrashIcon />
        Delete temporary files
      </button>
      {note && <span role="status">{note}</span>}
    </div>
  )
}
