import type { JobBoard, JobStatusKind } from '../../api'
import { ACCENTS, hashString, type Accent } from '../JobsPage/jobsTheme'

const NEUTRAL: Accent = { from: '#9ca3af', to: '#6b7280', soft: '#f3f4f6', text: '#4b5563' }

// The palette's violet, teal, sky and orange: the colours the fixed statuses (grey, green, rose, amber) do not use,
// so a profile's own column never looks like one of them.
const CUSTOM_ACCENTS: Accent[] = [ACCENTS[4], ACCENTS[5], ACCENTS[6], ACCENTS[7]]

// The colour of a column: fixed for the four fixed statuses, picked from the name for a profile's own (from colours the fixed ones leave free).
export function columnAccent(kind: JobStatusKind, name: string): Accent {
  switch (kind) {
    case 'not_reviewed':
      return NEUTRAL
    case 'applied':
      return ACCENTS[1]
    case 'declined':
      return ACCENTS[3]
    case 'stopped':
      return ACCENTS[2]
    case 'custom':
      return CUSTOM_ACCENTS[hashString(name.trim().toLowerCase()) % CUSTOM_ACCENTS.length]
  }
}

// How many jobs are on the board, over all its columns.
export function totalJobs(board: JobBoard): number {
  return board.columns.reduce((sum, c) => sum + c.jobs.length, 0)
}

// The id of the column the job is in on this board; undefined when it is not on it.
export function columnOf(board: JobBoard, jobId: string): string | undefined {
  return board.columns.find((c) => c.jobs.some((j) => j.jobId === jobId))?.id
}

// The board after a job moved to another column: it leads that column (the most recently moved comes first, like
// the backend lists it). The same board when the job is not on it or is already in that column.
export function moveCard(board: JobBoard, jobId: string, toColumnId: string, updatedAt: string): JobBoard {
  const card = board.columns.flatMap((c) => c.jobs).find((j) => j.jobId === jobId)
  const from = columnOf(board, jobId)
  if (!card || from === undefined || from === toColumnId || !board.columns.some((c) => c.id === toColumnId)) return board
  return {
    ...board,
    columns: board.columns.map((c) => {
      if (c.id === from) return { ...c, jobs: c.jobs.filter((j) => j.jobId !== jobId) }
      if (c.id === toColumnId) return { ...c, jobs: [{ ...card, updatedAt }, ...c.jobs] }
      return c
    }),
  }
}

// The board after a job came off it.
export function removeCard(board: JobBoard, jobId: string): JobBoard {
  return { ...board, columns: board.columns.map((c) => ({ ...c, jobs: c.jobs.filter((j) => j.jobId !== jobId) })) }
}

// The limits the backend enforces (jobboard.MaxCustomStatuses and MaxNameLength).
export const MAX_CUSTOM_STATUSES = 12
export const MAX_STATUS_NAME = 40

// How many statuses of its own the profile has on this board.
export function customCount(board: JobBoard): number {
  return board.columns.filter((c) => c.kind === 'custom').length
}

// Why a status name cannot be used, or '' when it can: blank, too long, or already a column of this board (the
// four fixed names included), ignoring case. exceptId is the status being renamed, which may keep or re-case its name.
export function statusNameProblem(board: JobBoard, name: string, exceptId?: string): string {
  const trimmed = name.trim()
  if (trimmed === '') return 'Give the status a name.'
  if (Array.from(trimmed).length > MAX_STATUS_NAME) return `A name can have at most ${String(MAX_STATUS_NAME)} characters.`
  const taken = board.columns.some((c) => c.id !== exceptId && c.name.toLowerCase() === trimmed.toLowerCase())
  return taken ? 'A column with this name already exists.' : ''
}

// The id of the fixed status a job starts in on a board (seeded by the backend).
export const NOT_REVIEWED_ID = 'fixed-not-reviewed'
