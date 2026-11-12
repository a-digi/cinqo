// The addresses of the unexpected stops (plan 259): the dashboard and the page of one stop (its UUID in the address). One place, so the dropdown, the menu, the
// routes and the table's links cannot drift apart.
export const UNEXPECTED_STOPS_PATH = '/admin/system/unexpected-stops'

export function unexpectedStopPath(id: string): string {
  return `${UNEXPECTED_STOPS_PATH}/${encodeURIComponent(id)}`
}
