// pieces.ts — the questions the code-driven import puts to a model, and the checks of its answers (plan 209, step 3).
//
// One builder for each thing the import needs. Each gives the Piece of askPiece: a prompt holding only that piece's own text and the JSON
// shape wanted, a check that holds the answer against the CV itself (a title the CV never uses, a skill that is not in its text, a
// company and title that do not add up to the header line are refused, which is what stops a small model's inventions), and what code can
// read from the CV when the model cannot. Where code reads it reliably (the name, the education lines) it is asked first and the model
// is not asked at all.

import type { Piece, Verdict } from './askPiece'
import { findDateRange, type CvJob, type SplitCv } from '../splitCv'

const ok = <T>(value: T): Verdict<T> => ({ ok: true, value })
const bad = (reason: string): Verdict<never> => ({ ok: false, reason })

const norm = (s: string): string => s.toLowerCase().replace(/\s+/g, ' ').trim()
const tokens = (s: string): string[] => norm(s).match(/[\p{L}\p{N}&+#]+/gu) ?? []

// The text given to a model is bounded: a small model drifts on a long one.
const clip = (s: string, max: number): string => (s.length <= max ? s : `${s.slice(0, max)} …`)
const block = (label: string, text: string): string => `${label}:\n"""\n${text}\n"""`

// The whole text of the CV, for checking that a value comes from it.
export function cvText(cv: SplitCv): string {
  return [...cv.header, ...cv.sections.flatMap((s) => [s.heading, ...s.lines]), ...cv.jobs.map((j) => j.header)].join('\n')
}

const appearsIn = (haystack: string, needle: string): boolean => norm(haystack).includes(norm(needle))

const PLACEHOLDER = /^(?:<.*>|.*\bthe person'?s\b.*|.*\b(?:your|full|first|last) name\b.*|john doe|jane doe|name|title|n\/?a|none|null|unknown|example|string)$/i

function text(value: unknown, field: string, max: number, min = 1): Verdict<string> {
  if (typeof value !== 'string') return bad(`"${field}" must be text`)
  const t = value.replace(/\s+/g, ' ').trim()
  if (t.length < min) return bad(`"${field}" is empty`)
  if (t.length > max) return bad(`"${field}" is too long (${String(t.length)} characters, at most ${String(max)})`)
  if (PLACEHOLDER.test(t)) return bad(`"${field}" is the example text, not the CV's own content`)
  return ok(t)
}

// The object of an answer. A model that is asked for an object sometimes wraps it in a list of one ([{"company": ...}]): that is read as the object.
const asObject = (parsed: unknown): Record<string, unknown> | null => {
  const value = Array.isArray(parsed) && parsed.length === 1 ? (parsed[0] as unknown) : parsed
  return typeof value === 'object' && value !== null && !Array.isArray(value) ? (value as Record<string, unknown>) : null
}

// --- the name -----------------------------------------------------------------------------------------------------------------------

// A name written all in capitals ("XHENI TAFA") is written as a name: "Xheni Tafa". A name that already has lower-case letters is left as it is.
function titleCaseIfShouted(name: string): string {
  if (name !== name.toLocaleUpperCase()) return name
  return name.toLocaleLowerCase().replace(/(^|[\s'-])(\p{L})/gu, (_m, before: string, letter: string) => before + letter.toLocaleUpperCase())
}

// The person's name read from the top lines: the first line of two to four words that are only letters (a name has no digit, no @, no colon).
export function nameFromHeader(header: string[]): string | null {
  for (const line of header) {
    const t = line.trim()
    if (/^\p{Lu}[\p{L}'.-]*(?:\s+\p{Lu}[\p{L}'.-]*){1,3}$/u.test(t)) return titleCaseIfShouted(t)
  }
  return null
}

// Asked only when nameFromHeader found none.
export function namePiece(cv: SplitCv): Piece<string> {
  const top = clip(cv.header.slice(0, 12).join('\n'), 600)
  return {
    prompt: `These are the first lines of a CV. ${block('Lines', top)}\nWhat is the person's name? Answer with this JSON and nothing else: {"name":"<the name as written above>"}`,
    validate: (parsed) => {
      const o = asObject(parsed)
      if (!o) return bad('it must be a JSON object {"name": ...}')
      const v = text(o.name, 'name', 60, 3)
      if (!v.ok) return v
      if (tokens(v.value).length > 5) return bad('"name" is more than five words, it is not a name')
      return appearsIn(top, v.value) ? v : bad('"name" is not in the lines above: copy the name as written')
    },
    fallback: () => nameFromHeader(cv.header),
  }
}

// --- the persona (a title the CV itself uses) --------------------------------------------------------------------------------------

// The profile text: the longest line of the profile sections (a paragraph).
function profileText(cv: SplitCv): string {
  const lines = cv.sections.filter((s) => s.kind === 'profile').flatMap((s) => s.lines)
  return lines.reduce((best, l) => (l.length > best.length ? l : best), '')
}

// A line such as "Senior Software Engineer | Tech lead | Team lead": the first part is the CV's own headline title.
function headlineTitle(cv: SplitCv): string | null {
  for (const line of [...cv.header, ...cv.sections.filter((s) => s.kind !== 'experience').flatMap((s) => s.lines)]) {
    if (line.includes(' | ') && line.length <= 120) {
      const first = (line.split(' | ')[0] ?? '').trim()
      if (first !== '' && first.split(/\s+/).length <= 6) return first
    }
  }
  return null
}

export interface PersonaValue {
  name: string
  title: string
}

export function personaPiece(cv: SplitCv): Piece<PersonaValue> {
  const titles = cv.jobs.map((j) => j.title ?? j.header).filter((t) => t !== '')
  const body = clip(profileText(cv), 1200)
  return {
    prompt: `${block('Profile of a CV', body)}\n${titles.length > 0 ? `${block('Job titles in the CV, the newest first', titles.slice(0, 6).join('\n'))}\n` : ''}Which job title describes this person now? Prefer the newest title that fits the profile, and use a title that is written in the CV. Answer with this JSON and nothing else: {"name":"<a short name for this career path>","title":"<the job title>"}`,
    validate: (parsed) => {
      const o = asObject(parsed)
      if (!o) return bad('it must be a JSON object {"name": ..., "title": ...}')
      const title = text(o.title, 'title', 80)
      if (!title.ok) return title
      if (tokens(title.value).length > 8 || /[.!?]$/.test(title.value)) return bad('"title" must be a short job title, not a sentence')
      if (/[,—–]/.test(title.value)) return bad('"title" is a whole line of the CV (an institution, a place, a dash): give only the job title, such as the part after the dash')
      if (!appearsIn(cvText(cv), title.value)) return bad(`"${title.value}" is not a title the CV uses: choose one that is written in the CV`)
      const name = o.name === undefined ? ok(title.value) : text(o.name, 'name', 60)
      if (!name.ok) return name
      if (tokens(name.value).length > 6) return bad('"name" must be a few words')
      return ok({ name: name.value, title: title.value })
    },
    fallback: () => {
      // Only a title the code is certain of: a header line (company, place, title in one) is no title.
      const title = headlineTitle(cv) ?? cv.jobs.find((j) => j.title !== null)?.title ?? null
      return title === null ? null : { name: title, title }
    },
  }
}

// --- the personal details --------------------------------------------------------------------------------------------------------

// "Anschrift : Hamburg, Deutschland LinkedIn : ..." gives "Hamburg, Deutschland".
export function locationFromText(t: string): string {
  const m = /(?:Anschrift|Adresse|Address|Wohnort|Location|Standort)\s*:\s*(.+?)(?=\s+(?:LinkedIn|Email|E-Mail|Tel\.?|Telefon|Phone|GitHub|Web|Xing)\b|\n|$)/i.exec(t)
  if (m?.[1]) return m[1].trim()
  // "Anschrift Langenfelder Damm 65, 22525 Hamburg": no colon, so only the city after a German postcode is taken (without one the text could be a sentence).
  const bare = /(?:Anschrift|Adresse|Wohnort)\s+(.+?)(?=\s+(?:LinkedIn|Email|E-Mail|Tel\.?|Telefon|Phone|GitHub|Web|Xing)\b|\n|$)/i.exec(t)
  const city = bare ? /\b\d{5}\s+(\p{Lu}[\p{L}.-]*(?:\s+\p{Lu}[\p{L}.-]*)*)/u.exec(bare[1]) : null
  return (city?.[1] ?? '').trim()
}

function firstSentences(t: string, count: number, max: number): string {
  const sentences = t.match(/[^.!?]+[.!?]+(?:\s|$)/g) ?? [t]
  return clip(sentences.slice(0, count).join('').trim(), max).replace(/ …$/, '')
}

export interface DetailsValue {
  headline: string
  summary: string
  location: string
}

export function detailsPiece(cv: SplitCv): Piece<DetailsValue> {
  const all = cvText(cv)
  const profile = clip(profileText(cv), 1500)
  const contact = cv.sections.filter((s) => s.kind === 'contact').flatMap((s) => s.lines)
  const top = clip([...cv.header.slice(0, 8), ...contact.slice(0, 6)].join('\n'), 700)
  const knownLocation = locationFromText(all)
  return {
    prompt: `${block('Top of a CV, with the contact lines', top)}\n${block('Profile of the CV', profile)}\nWrite the person's headline (one line), a summary (two or three sentences) and their city and country, using only what is written above and the same language as the CV text. Answer with this JSON and nothing else: {"headline":"<one line>","summary":"<two or three sentences>","location":"<city, country>"}`,
    validate: (parsed) => {
      const o = asObject(parsed)
      if (!o) return bad('it must be a JSON object {"headline": ..., "summary": ..., "location": ...}')
      const headline = text(o.headline, 'headline', 120, 3)
      if (!headline.ok) return headline
      if (String(o.headline).includes('\n')) return bad('"headline" must be one line')
      const summary = text(o.summary, 'summary', 700, 40)
      if (!summary.ok) return summary
      // A summary built from the CV uses the CV's words: most of its longer words are in the text.
      const long = tokens(summary.value).filter((w) => w.length >= 6)
      const known = long.filter((w) => norm(all).includes(w)).length
      if (long.length > 0 && known / long.length < 0.6) return bad('"summary" says things that are not in the CV: use only what is written')
      const loc = o.location === undefined || o.location === '' ? ok('') : text(o.location, 'location', 60)
      if (!loc.ok) return loc
      if (loc.value !== '' && !appearsIn(all, loc.value.split(',')[0] ?? loc.value)) return bad('"location" is not in the CV: give the place the CV states, or leave it empty')
      return ok({ headline: headline.value, summary: summary.value, location: loc.value })
    },
    fallback: () => {
      const headline = headlineTitle(cv) ?? cv.jobs[0]?.title ?? ''
      const summary = firstSentences(profileText(cv), 2, 600)
      if (headline === '' && summary === '' && knownLocation === '') return null
      return { headline, summary, location: knownLocation }
    },
  }
}

// --- the skills ---------------------------------------------------------------------------------------------------------------------

// The text a model reads the skills from: the skills sections, and the lines of a profile that list things (bullets, "Kernkompetenzen").
export function skillsText(cv: SplitCv): string {
  const lines: string[] = []
  // A headline ("Senior Software Engineer | Tech lead") and a sentence ("Fachliche Kompetenzen, die ich ... erworben habe.") are not skills.
  const keep = (l: string): boolean => !l.includes(' | ') && !(/[.!?]$/.test(l) && l.split(/\s+/).length >= 8)
  for (const s of cv.sections) {
    if (s.kind === 'skills') lines.push(...s.lines.filter(keep))
    if (s.kind === 'profile') {
      for (const l of s.lines) {
        const at = l.search(/Kernkompetenzen|Core competencies|Key skills/i)
        if (at >= 0) lines.push(l.slice(at))
        else if (/[●○•▪]/.test(l)) lines.push(l)
      }
    }
  }
  return lines.join('\n')
}

const MAX_SKILLS = 30

// The technologies the jobs list, most often first: clean comma lists straight from the CV.
export function skillsFromTechnologies(jobs: CvJob[]): string[] {
  const count = new Map<string, { label: string; n: number }>()
  for (const job of jobs) {
    for (const raw of job.technologies.split(/[,;]/)) {
      const label = raw.trim()
      if (label === '' || label.length > 40 || label.split(/\s+/).length > 4) continue
      const key = norm(label)
      const entry = count.get(key)
      if (entry) entry.n++
      else count.set(key, { label, n: 1 })
    }
  }
  return [...count.values()]
    .sort((a, b) => b.n - a.n)
    .slice(0, MAX_SKILLS)
    .map((e) => e.label)
}

// The titles the CV states (its headline parts and its job titles): a model lists them among the skills, they are not skills.
function titlesOf(cv: SplitCv): Set<string> {
  const out = new Set<string>()
  for (const line of cv.sections.filter((s) => s.kind !== 'experience').flatMap((s) => s.lines)) {
    if (line.includes(' | ') && line.length <= 120) for (const part of line.split(' | ')) out.add(norm(part))
  }
  for (const job of cv.jobs) if (job.title !== null) out.add(norm(job.title))
  return out
}

export function skillsPiece(cv: SplitCv): Piece<string[]> {
  const all = cvText(cv)
  const titles = titlesOf(cv)
  const source = clip(skillsText(cv), 2500)
  return {
    prompt: `${block('Skills text of a CV', source)}\nList the person's skills: one short name each (one to four words, such as "PHP" or "Domain Driven Design"), at most ${String(MAX_SKILLS)}, copied from the text. Answer with this JSON and nothing else: {"skills":["<skill 1>","<skill 2>"]}`,
    validate: (parsed) => {
      const list = Array.isArray(parsed) ? parsed : asObject(parsed)?.skills
      if (!Array.isArray(list)) return bad('it must be a JSON object {"skills": [...]}')
      const seen = new Set<string>()
      const skills: string[] = []
      for (const item of list) {
        if (typeof item !== 'string') continue
        const s = item.replace(/\s+/g, ' ').trim()
        if (s === '' || s.length > 40 || tokens(s).length > 4 || /[.!?]$/.test(s) || PLACEHOLDER.test(s)) continue
        if (!appearsIn(all, s) || seen.has(norm(s)) || titles.has(norm(s))) continue
        seen.add(norm(s))
        skills.push(s)
      }
      return skills.length >= 3 ? ok(skills.slice(0, MAX_SKILLS)) : bad('give at least three skills, each one to four words, copied from the text (a sentence is not a skill)')
    },
    fallback: () => {
      const fromJobs = skillsFromTechnologies(cv.jobs)
      return fromJobs.length >= 3 ? fromJobs : null
    },
  }
}

// --- company and title of a job whose header the CV does not make certain ------------------------------------------------------------

export interface CompanyTitle {
  company: string
  title: string
}

// The company and title must add up to the header line: the same words, nothing added, nothing dropped.
export function companyTitlePiece(header: string): Piece<CompanyTitle> {
  const want = tokens(header).sort().join(' ')
  return {
    prompt: `${block('Line of a CV', header)}\nThis line holds a company and a job title. Split it. Answer with this JSON and nothing else: {"company":"<the company>","title":"<the job title>"}`,
    validate: (parsed) => {
      const o = asObject(parsed)
      if (!o) return bad('it must be a JSON object {"company": ..., "title": ...}')
      const company = text(o.company, 'company', 80)
      if (!company.ok) return company
      const title = text(o.title, 'title', 80)
      if (!title.ok) return title
      // A job title is a role: a place ("4 Zeiten, Hamburg") or a dash is part of the company's line, not of the title.
      if (/[,—–]/.test(title.value) || /^\d/.test(title.value)) return bad('"title" is not a job title (it has a place, a dash or starts with a number): the title is only the role, the rest of the line is the company')
      const got = [...tokens(company.value), ...tokens(title.value)].sort().join(' ')
      return got === want ? ok({ company: company.value, title: title.value }) : bad('the company and the title together must be exactly the words of the line: add none and drop none')
    },
    fallback: () => (header.trim() === '' ? null : { company: header.trim(), title: 'Not stated' }),
  }
}

// --- the education ----------------------------------------------------------------------------------------------------------------

export interface EducationEntry {
  institution: string
  degree: string
  fieldOfStudy: string
  startDate: string
  endDate: string
}

// "2010 Bachelor of Informatik-Ingenieurwesen" or "2008 - 2010 Master Informatik": a line that begins with a year or a range of years. A single year
// is the year the degree was gained. The school is "Not stated" unless the CV names it (it never does on such a line).
const EDUCATION_LINE = /^(\d{4})(?:\s*[-–—]\s*(\d{4}|heute|aktuell|present|today))?\s+(\S.*)$/i

export function educationFromLines(lines: string[]): EducationEntry[] | null {
  const entries: EducationEntry[] = []
  for (const line of lines.map((l) => l.trim()).filter((l) => l !== '')) {
    const m = EDUCATION_LINE.exec(line)
    if (!m?.[1] || !m[3]) return null
    const range = m[2] as string | undefined
    const open = range !== undefined && /^(?:heute|aktuell|present|today)$/i.test(range)
    entries.push({
      institution: 'Not stated',
      degree: m[3].trim(),
      fieldOfStudy: '',
      startDate: range === undefined ? '' : m[1],
      endDate: range === undefined ? m[1] : open ? '' : range,
    })
  }
  return entries.length > 0 ? entries : null
}

// "Institution, Place — Degree October 2012 - Juli 2014", or the same with the date range on the line below ("Januar 2022 - Mai 2022"): every entry has its text and
// its dates, and the text is cut at the dash into the school and the degree. The text after a date range that opens a line (a remark in brackets) is left out. Null when
// a line does not fit (two lines of text in a row, text at the end with no date), so the model reads it instead.
export function educationFromDatedLines(lines: string[]): EducationEntry[] | null {
  const entries: EducationEntry[] = []
  let pending: string | null = null
  const entryOf = (text: string, d: { start: string; end: string }): EducationEntry => {
    const parts = text.split(/\s[—–]\s/)
    const school = parts.length === 2 ? (parts[0] ?? '').trim() : ''
    const degree = (parts.length === 2 ? (parts[1] ?? '') : text).trim()
    return { institution: school === '' ? 'Not stated' : school, degree, fieldOfStudy: '', startDate: d.start, endDate: d.end }
  }
  for (const line of lines.map((l) => l.trim()).filter((l) => l !== '')) {
    const d = findDateRange(line)
    if (!d) {
      if (pending !== null) return null
      pending = line
      continue
    }
    const before = line.slice(0, d.index).trim()
    if (before !== '') {
      if (pending !== null) return null
      entries.push(entryOf(before, d))
    } else {
      if (pending === null) return null
      entries.push(entryOf(pending, d))
      pending = null
    }
  }
  return pending === null && entries.length > 0 ? entries : null
}

export function educationText(cv: SplitCv): string {
  return cv.sections.filter((s) => s.kind === 'education').flatMap((s) => s.lines).join('\n')
}

const DATE_OK = /^\d{4}(?:-\d{2}(?:-\d{2})?)?$/

export function educationPiece(cv: SplitCv): Piece<EducationEntry[]> {
  const all = cvText(cv)
  const source = clip(educationText(cv), 1500)
  return {
    prompt: `${block('Education text of a CV', source)}\nList every school, degree, certificate or course in it. Dates are YYYY or YYYY-MM, empty when the text gives none; use "Not stated" for a school the text does not name. Answer with this JSON and nothing else: {"education":[{"institution":"<school>","degree":"<degree>","fieldOfStudy":"<field>","startDate":"<YYYY>","endDate":"<YYYY>"}]}`,
    validate: (parsed) => {
      // [{"education":[...]}] (the object in a list of one) is read as the object.
      const root = Array.isArray(parsed) && parsed.length === 1 && asObject(parsed[0])?.education !== undefined ? (parsed[0] as unknown) : parsed
      const list = Array.isArray(root) ? root : asObject(root)?.education
      if (!Array.isArray(list)) return bad('it must be a JSON object {"education": [...]}')
      const entries: EducationEntry[] = []
      for (const [i, item] of list.entries()) {
        const o = asObject(item)
        if (!o) return bad(`education[${String(i)}] must be an object`)
        const field = (key: string, max: number): string => (typeof o[key] === 'string' ? o[key].replace(/\s+/g, ' ').trim().slice(0, max) : '')
        const degree = field('degree', 120)
        let institution = field('institution', 120)
        if (institution === '' || /^(?:<.*>|not stated|unknown|n\/?a)$/i.test(institution)) institution = 'Not stated'
        const startDate = field('startDate', 10)
        const endDate = field('endDate', 10)
        if (degree === '' && institution === 'Not stated') return bad(`education[${String(i)}] has neither a degree nor a school`)
        if ((startDate !== '' && !DATE_OK.test(startDate)) || (endDate !== '' && !DATE_OK.test(endDate))) return bad(`education[${String(i)}]: dates are YYYY, YYYY-MM or empty`)
        if (startDate !== '' && endDate !== '' && endDate < startDate) return bad(`education[${String(i)}]: the end is before the start`)
        const named = [degree, institution === 'Not stated' ? '' : institution].filter((t) => t !== '')
        for (const t of named) if (!tokens(t).filter((w) => w.length >= 4).every((w) => norm(all).includes(w))) return bad(`education[${String(i)}] "${t}" is not in the CV: copy what is written`)
        entries.push({ institution, degree, fieldOfStudy: field('fieldOfStudy', 120), startDate, endDate })
      }
      return ok(entries.slice(0, 10))
    },
    fallback: () => {
      const lines = educationText(cv).split('\n')
      return educationFromLines(lines) ?? educationFromDatedLines(lines)
    },
  }
}
