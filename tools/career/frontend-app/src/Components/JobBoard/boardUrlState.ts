// The Job Board's card order in the address (step 136): the pure rules that turn an address into the board's state and a
// state back into its one canonical address.
//
//   /tools/career/job-board/<profileId>?sort=match
//
// The profile stays in the path. `moved` (most recently moved first) is the default and is left out; `match` (best match
// first) is written. Reading is forgiving: anything but exactly `match` is `moved`, the first value counts, unknown
// parameters are ignored.
import type { BoardSort } from '../../api'

export interface BoardUrlState {
  sort: BoardSort
}

export const DEFAULT_BOARD_STATE: BoardUrlState = { sort: 'moved' }

export function parseBoardUrl(search: string): BoardUrlState {
  return { sort: new URLSearchParams(search).get('sort') === 'match' ? 'match' : 'moved' }
}

// "" for the default order, "?sort=match" for best match first.
export function buildBoardSearch(state: BoardUrlState): string {
  return state.sort === 'match' ? '?sort=match' : ''
}

// True when the address is already exactly what the page would write for the state it means.
export function isCanonicalBoardSearch(search: string): boolean {
  const withMark = search !== '' && !search.startsWith('?') ? `?${search}` : search
  return buildBoardSearch(parseBoardUrl(search)) === withMark
}
