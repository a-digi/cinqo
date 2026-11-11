// cvSectionRules.ts (plan 251, step 3) — the pure parts of the wizard step "Education & more": the four sections a CV takes from the profile, how many entries each may
// have (the backend's limits, so the screen says no before the backend does), what a blank entry is, which field an entry cannot do without, and the one or two lines
// an entry is shown as. No React in here.
import type { CvCourseEntry, CvData, CvEducationEntry, CvHobbyEntry, CvLanguageEntry } from '../../api'

export type SectionKey = 'education' | 'courses' | 'languages' | 'hobbies'

export interface SectionField<T> {
  key: keyof T & string
  label: string
  multiline?: boolean
}

export interface SectionSpec<T> {
  key: SectionKey
  title: string
  // the backend's limit for this section (cvbuilder/validate_sections.go)
  max: number
  blank: T
  // the field an entry cannot be saved without, and what to call an entry in the messages
  required: keyof T & string
  noun: string
  fields: SectionField<T>[]
  // what the list shows for an entry: a main line and a smaller one
  summary: (e: T) => { main: string; sub: string }
  emptyText: string
}

const join = (parts: string[], by: string) => parts.filter((p) => p.trim() !== '').join(by)

// "2010 – 2012", "2020 – Present" (no end date: still studying), or just the end when there is no start; nothing when there is neither.
export function dateRange(start: string, end: string): string {
  if (start.trim() !== '') return `${start} – ${end.trim() !== '' ? end : 'Present'}`
  return end
}

export const EDUCATION: SectionSpec<CvEducationEntry> = {
  key: 'education',
  title: 'Education',
  max: 20,
  blank: { institution: '', degree: '', fieldOfStudy: '', startDate: '', endDate: '', description: '' },
  required: 'institution',
  noun: 'education entry',
  fields: [
    { key: 'institution', label: 'Institution' },
    { key: 'degree', label: 'Degree' },
    { key: 'fieldOfStudy', label: 'Field of study' },
    { key: 'startDate', label: 'Start' },
    { key: 'endDate', label: 'End (empty: still studying)' },
    { key: 'description', label: 'Description', multiline: true },
  ],
  summary: (e) => ({
    main: e.institution || 'Untitled',
    sub: join([join([e.degree, e.fieldOfStudy], ', '), dateRange(e.startDate, e.endDate)], ' · '),
  }),
  emptyText: 'No education on this CV.',
}

export const COURSES: SectionSpec<CvCourseEntry> = {
  key: 'courses',
  title: 'Courses',
  max: 30,
  blank: { title: '', provider: '', completedDate: '' },
  required: 'title',
  noun: 'course',
  fields: [
    { key: 'title', label: 'Title' },
    { key: 'provider', label: 'Provider' },
    { key: 'completedDate', label: 'Completed' },
  ],
  summary: (e) => ({ main: e.title || 'Untitled', sub: join([e.provider, e.completedDate], ' · ') }),
  emptyText: 'No courses on this CV.',
}

export const LANGUAGES: SectionSpec<CvLanguageEntry> = {
  key: 'languages',
  title: 'Languages',
  max: 20,
  blank: { language: '', level: '' },
  required: 'language',
  noun: 'language',
  fields: [
    { key: 'language', label: 'Language' },
    { key: 'level', label: 'Level (Native, C1, B2…)' },
  ],
  summary: (e) => ({ main: e.language || 'Untitled', sub: e.level }),
  emptyText: 'No languages on this CV.',
}

export const HOBBIES: SectionSpec<CvHobbyEntry> = {
  key: 'hobbies',
  title: 'Hobbies',
  max: 30,
  blank: { name: '' },
  required: 'name',
  noun: 'hobby',
  fields: [{ key: 'name', label: 'Hobby' }],
  summary: (e) => ({ main: e.name || 'Untitled', sub: '' }),
  emptyText: 'No hobbies on this CV.',
}

// An entry can be saved when its required field has something in it (an entry with only a date is nothing).
export function canSave<T>(spec: SectionSpec<T>, entry: T): boolean {
  return String(entry[spec.required]).trim() !== ''
}

// Whether the section has room for one more entry.
export function canAdd(spec: { max: number }, count: number): boolean {
  return count < spec.max
}

// The review line: what the CV will hold, "3 education entries, 1 course, 2 languages", the sections without entries left out ('' when there are none).
export function sectionsSummary(data: CvData): string {
  const part = (n: number, one: string, many: string) => (n === 0 ? '' : `${String(n)} ${n === 1 ? one : many}`)
  return join(
    [
      part(data.education?.length ?? 0, 'education entry', 'education entries'),
      part(data.courses?.length ?? 0, 'course', 'courses'),
      part(data.languages?.length ?? 0, 'language', 'languages'),
      part(data.hobbies?.length ?? 0, 'hobby', 'hobbies'),
    ],
    ' · ',
  )
}
