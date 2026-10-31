// Pure rules for the persona view page (step 118). No React, so they can be script-checked.

interface DetailsLike {
  headline: string
  summary: string
  location: string
  desiredTitles: string
  desiredLocations: string
  minSalary: number
}

export interface DetailRow {
  label: string
  // '' means nothing was filled in (the page prints "Not set").
  value: string
}

// The summary-and-goals fields in the order the page shows them (the long summary is shown apart, as text).
// The minimum salary is empty for 0 (nothing given).
export function detailRows(d: DetailsLike | null): DetailRow[] {
  return [
    { label: 'Headline', value: d?.headline.trim() ?? '' },
    { label: 'Location', value: d?.location.trim() ?? '' },
    { label: 'Desired titles', value: d?.desiredTitles.trim() ?? '' },
    { label: 'Desired locations', value: d?.desiredLocations.trim() ?? '' },
    { label: 'Minimum salary', value: d && d.minSalary > 0 ? d.minSalary.toLocaleString('en-US') : '' },
  ]
}

// True when nothing at all is filled in (no field, no summary): the page then says so and points at the
// details page.
export function nothingFilledIn(d: DetailsLike | null): boolean {
  return detailRows(d).every((r) => r.value === '') && (d?.summary.trim() ?? '') === ''
}

// "Jan 2020 to present", "… to Mar 2022", "Jan 2020 to Mar 2022"; '' when neither date is given.
export function rolePeriod(startDate: string, endDate: string): string {
  const start = startDate.trim()
  const end = endDate.trim()
  if (start === '' && end === '') return ''
  return `${start === '' ? '…' : start} to ${end === '' ? 'present' : end}`
}

// The server leaves out the empty dates and description of an experience entry (omitempty), although the
// shared type says they are strings. This takes the wider shape the server really sends and returns strings,
// so the page never calls a string method on a missing field.
export interface RoleLike {
  id: string
  company: string
  title: string
  startDate?: string
  endDate?: string
  description?: string
}

export interface RoleView {
  id: string
  company: string
  title: string
  period: string
  description: string
}

export function roleView(r: RoleLike): RoleView {
  return {
    id: r.id,
    company: r.company,
    title: r.title,
    period: rolePeriod(r.startDate ?? '', r.endDate ?? ''),
    description: (r.description ?? '').trim(),
  }
}
