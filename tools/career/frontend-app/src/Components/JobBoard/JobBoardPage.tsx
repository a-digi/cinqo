import { useEffect, useState } from 'react'
import {
  createJobStatus,
  deleteJobStatus,
  fetchJobBoard,
  fetchProfiles,
  removeJobBoardEntry,
  renameJobStatus,
  setJobBoardEntry,
  type JobBoard,
  type JobBoardCard,
  type JobBoardColumn,
  type Profile,
} from '../../api'
import { ActionMenu, type ActionMenuItem } from '../../Shared/ActionMenu/ActionMenu'
import { AppLink } from '../../Shared/AppLink/AppLink'
import { ConfirmationModal } from '../../Shared/ConfirmationModal/ConfirmationModal'
import { BriefcaseIcon, PlusIcon, TrashIcon } from '../../Shared/Icons/icons'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { JOB_DETAILS_PATH, PROFILES_PATH, VIEW_PROFILE_PATH } from '../../paths'
import { plural } from '../CompaniesPage/companyStats'
import { PAGE_ACCENT } from '../JobsPage/jobsTheme'
import { profileLabel } from '../ProfileSwitcher/profileLabel'
import {
  MAX_CUSTOM_STATUSES,
  columnAccent,
  columnOf,
  customCount,
  moveCard,
  removeCard,
  statusNameProblem,
  totalJobs,
} from './jobBoardRules'
import { StatusNameForm } from './StatusNameForm'

// The drag payload's type: a job id, only ever read back by this page.
const DRAG_TYPE = 'application/x-career-job'

// One profile's job board (/tools/career/job-board/:profileId): the statuses as columns, each with the jobs
// that profile has in it. A job moves to another column by drag and drop, or with "Move to" in its menu (which
// also works without a mouse and on a touch screen, where the browser's own drag does not); "Remove from board"
// takes it off. A move shows at once and the board is read again if the save fails. The profile's own statuses are
// columns too: "Add status" at the end of the row adds one, its menu renames or deletes it (its jobs go back to Not
// Reviewed). Adding jobs to the board comes in the step after this one. See plan/ai/tools/career/step-126-job-board-statuses.md.
export function JobBoardPage({ profileId }: { profileId: string }) {
  const [profile, setProfile] = useState<Profile | null>(null)
  const [board, setBoard] = useState<JobBoard | null>(null)
  const [error, setError] = useState('')
  const [loaded, setLoaded] = useState(false)
  // The card being dragged and the column it is over (to light that column up).
  const [draggingId, setDraggingId] = useState<string | null>(null)
  const [overColumnId, setOverColumnId] = useState<string | null>(null)
  const [pendingRemove, setPendingRemove] = useState<JobBoardCard | null>(null)
  const [pendingDelete, setPendingDelete] = useState<JobBoardColumn | null>(null)
  const [adding, setAdding] = useState(false)

  useEffect(() => {
    setLoaded(false)
    setError('')
    Promise.all([fetchProfiles(), fetchJobBoard(profileId)])
      .then(([profiles, loadedBoard]) => {
        setProfile(profiles.find((p) => p.id === profileId) ?? null)
        setBoard(loadedBoard)
      })
      .catch((err: unknown) => {
        setBoard(null)
        setError(
          err instanceof Error && err.message.includes('(404)') ? 'Profile not found' : err instanceof Error ? err.message : String(err),
        )
      })
      .finally(() => {
        setLoaded(true)
      })
  }, [profileId])

  // After a failed save the board is read again, so it shows what is really stored.
  function reloadAfterFailure(err: unknown) {
    setError(err instanceof Error ? err.message : String(err))
    fetchJobBoard(profileId)
      .then(setBoard)
      .catch(() => undefined)
  }

  function moveJob(jobId: string, toColumnId: string) {
    if (!board || columnOf(board, jobId) === undefined) return
    setError('')
    setBoard(moveCard(board, jobId, toColumnId, new Date().toISOString()))
    setJobBoardEntry(profileId, jobId, toColumnId).catch(reloadAfterFailure)
  }

  function removeJob(jobId: string) {
    if (!board) return
    setError('')
    setBoard(removeCard(board, jobId))
    removeJobBoardEntry(profileId, jobId).catch(reloadAfterFailure)
  }

  // The statuses: after a change the board is read again (the new column's place and a deleted column's jobs
  // come from the server). A refused name throws, for the form to show.
  async function addStatus(name: string) {
    await createJobStatus(profileId, name)
    setBoard(await fetchJobBoard(profileId))
    setAdding(false)
  }

  async function renameStatus(id: string, name: string) {
    await renameJobStatus(id, name)
    setBoard(await fetchJobBoard(profileId))
  }

  function deleteStatus(id: string) {
    setError('')
    deleteJobStatus(id)
      .then(() => fetchJobBoard(profileId))
      .then(setBoard)
      .catch(reloadAfterFailure)
  }

  const profileHref = `${VIEW_PROFILE_PATH}?id=${encodeURIComponent(profileId)}`
  const backLink = (
    <AppLink href={profile ? profileHref : PROFILES_PATH} className="mb-3 inline-block text-sm font-medium text-blue-700 hover:underline">
      {profile ? `← ${profileLabel(profile)}` : '← All profiles'}
    </AppLink>
  )

  if (!board || !profile) {
    return (
      <div className="max-w-3xl p-6 font-sans text-gray-900">
        {backLink}
        <div className="text-sm text-gray-500">{loaded ? error || 'Profile not found' : 'Loading…'}</div>
      </div>
    )
  }

  const total = totalJobs(board)
  return (
    <div className="career-v-tilt career-a-float career-t-wave max-w-full p-6 font-sans text-gray-900">
      {backLink}
      <PageHeaderCard
        icon={<BriefcaseIcon />}
        title="Job board"
        description={`The jobs ${profileLabel(profile)} is working on, by status. Drag a job to another column, or use its menu.`}
        pill={{ text: plural(total, 'job', 'jobs') }}
      />
      {error !== '' && (
        <p role="alert" className="mb-3 text-sm text-red-700">
          {error}
        </p>
      )}
      {total === 0 && (
        <p className="mb-4 rounded-3xl border border-gray-100 bg-gray-50 p-6 text-center text-sm text-gray-500">
          No jobs on this board yet. A job shows here once it has been added to it.
        </p>
      )}
      <div className="flex items-start gap-4 overflow-x-auto pb-4" role="list" aria-label="Statuses">
        {board.columns.map((column) => (
          <Column
            key={column.id}
            column={column}
            columns={board.columns}
            draggingId={draggingId}
            isOver={overColumnId === column.id}
            onDragOver={setOverColumnId}
            onDropJob={(jobId) => {
              setOverColumnId(null)
              setDraggingId(null)
              moveJob(jobId, column.id)
            }}
            onDragStartJob={setDraggingId}
            onDragEndJob={() => {
              setDraggingId(null)
              setOverColumnId(null)
            }}
            onMove={moveJob}
            onRemove={setPendingRemove}
            validateName={(name) => statusNameProblem(board, name, column.id)}
            onRename={(name) => renameStatus(column.id, name)}
            onDelete={setPendingDelete}
          />
        ))}
        <AddStatus
          atLimit={customCount(board) >= MAX_CUSTOM_STATUSES}
          adding={adding}
          onStart={() => {
            setAdding(true)
          }}
          onCancel={() => {
            setAdding(false)
          }}
          validateName={(name) => statusNameProblem(board, name)}
          onAdd={addStatus}
        />
      </div>
      <ConfirmationModal
        open={pendingDelete !== null}
        title="Delete status"
        message={
          pendingDelete
            ? pendingDelete.jobs.length === 0
              ? `Delete the status "${pendingDelete.name}"?`
              : `Delete the status "${pendingDelete.name}"? ${pendingDelete.jobs.length === 1 ? 'Its job goes' : `Its ${String(pendingDelete.jobs.length)} jobs go`} back to Not Reviewed.`
            : ''
        }
        confirmLabel="Delete"
        variant="danger"
        onConfirm={() => {
          if (pendingDelete) deleteStatus(pendingDelete.id)
          setPendingDelete(null)
        }}
        onCancel={() => {
          setPendingDelete(null)
        }}
      />
      <ConfirmationModal
        open={pendingRemove !== null}
        title="Remove from board"
        message={pendingRemove ? `Remove "${pendingRemove.title}" from this board? The job itself is kept.` : ''}
        confirmLabel="Remove"
        variant="danger"
        onConfirm={() => {
          if (pendingRemove) removeJob(pendingRemove.jobId)
          setPendingRemove(null)
        }}
        onCancel={() => {
          setPendingRemove(null)
        }}
      />
    </div>
  )
}

function Column({
  column,
  columns,
  draggingId,
  isOver,
  onDragOver,
  onDropJob,
  onDragStartJob,
  onDragEndJob,
  onMove,
  onRemove,
  validateName,
  onRename,
  onDelete,
}: {
  column: JobBoardColumn
  columns: JobBoardColumn[]
  draggingId: string | null
  isOver: boolean
  onDragOver: (columnId: string | null) => void
  onDropJob: (jobId: string) => void
  onDragStartJob: (jobId: string) => void
  onDragEndJob: () => void
  onMove: (jobId: string, toColumnId: string) => void
  onRemove: (job: JobBoardCard) => void
  validateName: (name: string) => string
  onRename: (name: string) => Promise<void>
  onDelete: (column: JobBoardColumn) => void
}) {
  const accent = columnAccent(column.kind, column.name)
  const [renaming, setRenaming] = useState(false)
  return (
    <section
      role="listitem"
      aria-label={column.name}
      data-over={isOver ? 'true' : undefined}
      onDragOver={(e) => {
        // Only a job card dragged from this board may be dropped here (not a file or a piece of text).
        if (!e.dataTransfer.types.includes(DRAG_TYPE)) return
        e.preventDefault()
        e.dataTransfer.dropEffect = 'move'
        onDragOver(column.id)
      }}
      onDragLeave={(e) => {
        // Leaving for one of the column's own children is not leaving the column.
        if (!e.currentTarget.contains(e.relatedTarget as Node | null)) onDragOver(null)
      }}
      onDrop={(e) => {
        const jobId = e.dataTransfer.getData(DRAG_TYPE)
        if (jobId === '') return
        e.preventDefault()
        onDropJob(jobId)
      }}
      className="w-72 shrink-0 rounded-3xl border p-3 shadow-sm transition-shadow"
      style={{
        borderColor: isOver ? accent.to : accent.soft,
        background: accent.soft,
        boxShadow: isOver ? `0 0 0 3px ${accent.from}` : undefined,
      }}
    >
      {renaming ? (
        <div className="mb-3">
          <StatusNameForm
            initial={column.name}
            label={`New name for ${column.name}`}
            submitLabel="Rename"
            validate={validateName}
            onSubmit={async (name) => {
              await onRename(name)
              setRenaming(false)
            }}
            onCancel={() => {
              setRenaming(false)
            }}
          />
        </div>
      ) : (
        <div className="mb-3 flex items-center gap-2 px-1">
          <h2 className="flex min-w-0 flex-1 items-center gap-2 text-sm font-semibold" style={{ color: accent.text }}>
            <span className="h-2.5 w-2.5 shrink-0 rounded-full" style={{ background: accent.to }} aria-hidden="true" />
            <span className="min-w-0 flex-1 truncate">{column.name}</span>
            <span className="rounded-full bg-white/80 px-2 py-0.5 text-xs font-semibold">{column.jobs.length}</span>
          </h2>
          {column.kind === 'custom' && (
            <ActionMenu
              triggerLabel={`Actions for the status ${column.name}`}
              items={[
                {
                  key: 'rename',
                  label: 'Rename',
                  onClick: () => {
                    setRenaming(true)
                  },
                },
                {
                  key: 'delete',
                  label: 'Delete status',
                  icon: <TrashIcon />,
                  variant: 'danger',
                  onClick: () => {
                    onDelete(column)
                  },
                },
              ]}
            />
          )}
        </div>
      )}
      {column.jobs.length === 0 ? (
        <p className="rounded-2xl border border-dashed border-gray-300 px-3 py-6 text-center text-xs text-gray-400">
          {draggingId !== null ? 'Drop here' : 'No jobs'}
        </p>
      ) : (
        <ul className="space-y-2">
          {column.jobs.map((job) => {
            const moveItems: ActionMenuItem[] = columns
              .filter((c) => c.id !== column.id)
              .map((c) => ({
                key: c.id,
                label: c.name,
                onClick: () => {
                  onMove(job.jobId, c.id)
                },
              }))
            const menuItems: ActionMenuItem[] = [
              { key: 'move', label: 'Move to…', children: moveItems },
              {
                key: 'remove',
                label: 'Remove from board',
                icon: <TrashIcon />,
                variant: 'danger',
                onClick: () => {
                  onRemove(job)
                },
              },
            ]
            return (
              // No transform on the card (not even on hover): it would become the containing block of the menu's
              // fixed position and send the menu off-screen.
              <li
                key={job.jobId}
                draggable
                onDragStart={(e) => {
                  e.dataTransfer.setData(DRAG_TYPE, job.jobId)
                  e.dataTransfer.effectAllowed = 'move'
                  onDragStartJob(job.jobId)
                }}
                onDragEnd={onDragEndJob}
                className={`cursor-grab rounded-2xl border border-gray-100 bg-white p-3 shadow-sm transition-shadow hover:shadow-md active:cursor-grabbing ${
                  draggingId === job.jobId ? 'opacity-40' : ''
                }`}
              >
                <div className="flex items-start gap-1">
                  <div className="min-w-0 flex-1">
                    <AppLink
                      href={`${JOB_DETAILS_PATH}?id=${encodeURIComponent(job.jobId)}`}
                      className="block text-sm font-semibold leading-snug hover:underline"
                      style={{ color: PAGE_ACCENT.text }}
                    >
                      {job.title}
                    </AppLink>
                    {job.company !== '' && <p className="mt-0.5 truncate text-xs text-gray-600">{job.company}</p>}
                    {job.location !== '' && <p className="truncate text-xs text-gray-400">{job.location}</p>}
                  </div>
                  <ActionMenu items={menuItems} triggerLabel={`Actions for ${job.title}`} />
                </div>
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}

// The last item of the row: the way to add a status of the profile's own, up to the limit.
function AddStatus({
  atLimit,
  adding,
  onStart,
  onCancel,
  validateName,
  onAdd,
}: {
  atLimit: boolean
  adding: boolean
  onStart: () => void
  onCancel: () => void
  validateName: (name: string) => string
  onAdd: (name: string) => Promise<void>
}) {
  return (
    <div role="listitem" aria-label="Add status" className="w-72 shrink-0 rounded-3xl border border-dashed border-gray-300 p-3">
      {adding ? (
        <StatusNameForm label="Name of the new status" submitLabel="Add" validate={validateName} onSubmit={onAdd} onCancel={onCancel} />
      ) : atLimit ? (
        <p className="px-1 py-2 text-xs text-gray-500">
          This board has the most own statuses it can have ({MAX_CUSTOM_STATUSES}). Delete one to add another.
        </p>
      ) : (
        <button
          type="button"
          onClick={onStart}
          className="flex w-full items-center justify-center gap-1.5 rounded-2xl px-3 py-2 text-sm font-medium text-gray-600 hover:bg-gray-50"
        >
          <PlusIcon />
          Add status
        </button>
      )}
    </div>
  )
}
