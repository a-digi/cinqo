// splitCv.ts — cuts the text of a CV into its parts in code (plan 209, step 2).
//
// The code-driven import (for a model that cannot take a whole CV) gives the model one small piece at a time. This is the part that decides
// what the pieces are: the header, the sections found by their headings (English and German), and the jobs of the experience section with
// their dates, bullet points and technologies. It is a pure function of the text; it never asks a model and never guesses a fact it
// cannot read: a company that cannot be told from a title is left null, for the caller to ask about.
//
// The text it is made for is what pdf_to_markdown gives: one line per layout line, columns interleaved, a job's bullet points often on one
// line separated by "●". Where a sidebar's fragments end up inside a job ("Message Broker ● RabbitMQ"), they are kept apart as `stray`
// lines instead of becoming bullet points of the job.

export type SectionKind = 'contact' | 'profile' | 'skills' | 'experience' | 'education' | 'languages' | 'other'

export interface CvSection {
  kind: SectionKind
  heading: string
  lines: string[]
}

export interface CvJob {
  // The line with the company and the title, as the CV writes it ('' when it could not be told).
  header: string
  // header cut in two where the CV makes it certain (a legal form such as GmbH, or a separator next to one); null when it is not certain.
  company: string | null
  title: string | null
  // YYYY-MM or YYYY; endDate is '' for a job that is still going on.
  startDate: string
  endDate: string
  current: boolean
  // The sentence(s) before the first bullet point (without the location), the bullet points, what looked like a sidebar's fragments, and the
  // technology list.
  intro: string
  bullets: string[]
  stray: string[]
  technologies: string
}

export interface SplitCv {
  // The lines before the first heading: the name and whatever the CV puts on top.
  header: string[]
  sections: CvSection[]
  jobs: CvJob[]
  // False when no heading of a profile, skills, experience or education section was found: the text is not one this can be cut.
  usable: boolean
}

const HEADINGS: Record<Exclude<SectionKind, 'other'>, string[]> = {
  contact: ['kontakt', 'contact', 'contact details', 'kontaktdaten'],
  profile: ['profil', 'profile', 'kurzprofil', 'summary', 'professional summary', 'about me', 'über mich', 'zusammenfassung', 'personal statement'],
  skills: [
    'skills', 'key skills', 'technical skills', 'core competencies', 'kernkompetenzen', 'kenntnisse', 'it kenntnisse', 'it-kenntnisse', 'edv kenntnisse',
    'kompetenzen', 'fähigkeiten', 'berufliche fähigkeiten', 'fachliche kompetenzen', 'expertise', 'technologien', 'technologies',
  ],
  experience: [
    'berufserfahrung', 'work experience', 'experience', 'professional experience', 'beruflicher werdegang', 'werdegang', 'employment history',
    'work history', 'career history', 'berufliche erfahrung', 'praxiserfahrung',
  ],
  education: [
    'ausbildung', 'education', 'studium', 'abschlüsse', 'qualifikationen', 'bildung', 'akademischer werdegang', 'schulbildung', 'weiterbildung',
    'zertifikate', 'certifications', 'certificates', 'zertifizierungen', 'kurse', 'education and training', 'ausbildung und weiterbildung',
  ],
  languages: ['sprachen', 'languages', 'sprachkenntnisse'],
}
const OTHER_HEADINGS = ['projekte', 'projects', 'interessen', 'interests', 'hobbies', 'hobbys', 'publikationen', 'publications', 'referenzen', 'references', 'ehrenamt', 'volunteering', 'auszeichnungen', 'awards']

const HEADING_KIND = new Map<string, SectionKind>()
for (const [kind, words] of Object.entries(HEADINGS)) for (const w of words) HEADING_KIND.set(w, kind as SectionKind)
for (const w of OTHER_HEADINGS) HEADING_KIND.set(w, 'other')

const BULLET_CHARS = '●•▪◦○■◆▶'
const BULLET = new RegExp(`[${BULLET_CHARS}]`)
const BULLET_SPLIT = new RegExp(`\\s*[${BULLET_CHARS}]\\s*`)

// The kind of section a line opens, or null when the line is not a heading: a short line that is nothing but a known heading word.
export function headingKind(line: string): SectionKind | null {
  const words = line
    .toLowerCase()
    .replace(/[:·|]+\s*$/, '')
    .replace(/^[^a-zäöüß]+|[^a-zäöüß]+$/g, '')
    .trim()
  if (words === '' || words.split(/\s+/).length > 4) return null
  return HEADING_KIND.get(words.replace(/\s+/g, ' ')) ?? null
}

const MONTHS: Record<string, string> = {
  jan: '01', januar: '01', january: '01', feb: '02', februar: '02', february: '02', mär: '03', mar: '03', märz: '03', march: '03', apr: '04', april: '04',
  mai: '05', may: '05', jun: '06', juni: '06', june: '06', jul: '07', juli: '07', july: '07', aug: '08', august: '08', sep: '09', sept: '09', september: '09',
  okt: '10', oct: '10', oktober: '10', october: '10', nov: '11', november: '11', dez: '12', dec: '12', dezember: '12', december: '12',
}

const MONTH_NAMES = Object.keys(MONTHS).sort((x, y) => y.length - x.length).join('|')
const DATE_PART = String.raw`(?:\d{1,2}[./]\d{4}|(?:${MONTH_NAMES})\.?\s+\d{4}|\d{4})`
const NOW_WORDS = String.raw`bis heute|aktuell|heute|present|now|current|jetzt|today|ongoing|laufend`
const DATE_RANGE = new RegExp(String.raw`(?<![\w])(?<start>${DATE_PART})\s*[-–—]\s*(?<end>${DATE_PART}|${NOW_WORDS})(?!\w)`, 'i')

// One date as YYYY-MM (or YYYY), '' when it is not a date.
export function parseCvDate(text: string): string {
  const t = text.trim()
  let m = /^(\d{1,2})[./](\d{4})$/.exec(t)
  if (m?.[1] && m[2]) {
    const month = Number(m[1])
    return month >= 1 && month <= 12 ? `${m[2]}-${String(month).padStart(2, '0')}` : ''
  }
  m = /^([A-Za-zÄÖÜäöü]{3,9})\.?\s+(\d{4})$/.exec(t)
  if (m?.[1] && m[2]) {
    const month = MONTHS[m[1].toLowerCase()]
    return month ? `${m[2]}-${month}` : ''
  }
  return /^\d{4}$/.test(t) ? t : ''
}

const COUNTRY = /\b(Deutschland|Germany|Albanien|Albania|Österreich|Austria|Schweiz|Switzerland|USA|UK|England|Frankreich|France|Niederlande|Netherlands|Kanada|Canada)\b/gi
const TECH_MARK = /(?:^|\s)[>›]?\s*((?:eingesetzte\s+)?technologien|technologies|tech\s*stack)\s*:\s*/i
const LEGAL = String.raw`(?:GmbH\s*&\s*Co\.?\s*KG|GmbH|AG|SE|KG|UG|Inc\.?|Ltd\.?|LLC|Corp\.?|Co\.|S\.A\.|B\.V\.|Oy|AB)`
const COMPANY_FIRST = new RegExp(String.raw`^(.*?\b${LEGAL})(?=\s|$|[/|,–—-])\s*[/|,–—-]*\s*(.*)$`)
const COMPANY_LAST = new RegExp(String.raw`^(.*?)\s+(?:@|bei|at)\s+(.*\b${LEGAL})$`, 'i')

// The header cut into company and title where the CV makes it certain: the company ends at a legal form ("Statista GmbH Team Lead"), or
// follows "@", "bei", "at". Anything else (no legal form: "MCN Softwareentwickler") is null, null.
export function splitCompanyTitle(header: string): { company: string | null; title: string | null } {
  const h = header.trim()
  const last = COMPANY_LAST.exec(h)
  if (last?.[1] && last[2]) return { company: last[2].trim(), title: last[1].trim() }
  const first = COMPANY_FIRST.exec(h)
  const title = first?.[2]?.trim() ?? ''
  if (first?.[1] && title !== '') return { company: first[1].trim(), title }
  return { company: null, title: null }
}

interface Block {
  header: string
  startDate: string
  endDate: string
  current: boolean
  lines: string[]
}

function dateOf(line: string): { index: number; length: number; start: string; end: string; current: boolean } | null {
  const m = DATE_RANGE.exec(line)
  if (!m?.groups || m.index > 140) return null
  const start = parseCvDate(m.groups.start)
  if (start === '') return null
  const endText = m.groups.end.trim()
  const current = new RegExp(`^(?:${NOW_WORDS})$`, 'i').test(endText)
  const end = current ? '' : parseCvDate(endText)
  if (!current && end === '') return null
  return { index: m.index, length: m[0].length, start, end, current }
}

// The experience lines cut into one block per job. A job starts on the line with its date range: when the line holds the company and title
// before the date, that is the header; when the date opens the line, the line before is.
function jobBlocks(lines: string[]): Block[] {
  const starts: { line: number; headerLine: number; date: NonNullable<ReturnType<typeof dateOf>> }[] = []
  lines.forEach((line, i) => {
    const date = dateOf(line)
    if (!date) return
    const before = line.slice(0, date.index).trim()
    if (before !== '' && !BULLET.test(before) && before.length <= 140) {
      starts.push({ line: i, headerLine: i, date })
      return
    }
    if (before === '') {
      const prev = i > 0 ? lines[i - 1] : ''
      const usable = prev !== '' && !BULLET.test(prev) && prev.length <= 140 && !dateOf(prev)
      starts.push({ line: i, headerLine: usable ? i - 1 : i, date })
    }
  })
  return starts.map((s, k) => {
    const end = k + 1 < starts.length ? starts[k + 1].headerLine : lines.length
    const line = lines[s.line]
    const header = s.headerLine === s.line ? line.slice(0, s.date.index).trim() : lines[s.headerLine].trim()
    const rest = line.slice(s.date.index + s.date.length)
    return { header, startDate: s.date.start, endDate: s.date.end, current: s.date.current, lines: [rest, ...lines.slice(s.line + 1, end)] }
  })
}

const SENTENCE_END = /[.!?;:)%"”]\s*$/

// A block's lines read into an introduction, bullet points, stray fragments and the technology list.
function readBlock(block: Block): Pick<CvJob, 'intro' | 'bullets' | 'stray' | 'technologies'> {
  let intro = ''
  let technologies = ''
  const bullets: string[] = []
  const stray: string[] = []
  let seenBullet = false
  const state = { techNext: false }
  let first = true

  const takeTech = (text: string): string => {
    const m = TECH_MARK.exec(text)
    if (!m) return text
    const after = text.slice(m.index + m[0].length).trim()
    technologies = after
    state.techNext = after === ''
    return text.slice(0, m.index).trim()
  }

  for (let raw of block.lines) {
    raw = raw.trim()
    if (first) {
      first = false
      const afterComma = raw.startsWith(',')
      raw = raw.replace(/^[,;\s]+/, '')
      // The location follows the date ("München (remote), Deutschland Technische ..."): cut after the last country in the first words.
      const countries = [...raw.slice(0, 90).matchAll(COUNTRY)]
      const lastCountry = countries[countries.length - 1]
      if (countries.length > 0) raw = raw.slice(lastCountry.index + lastCountry[0].length).replace(/^[)\]\s,;:.–—-]+/, '')
      else if (afterComma) {
        // No country named: a few words after the comma and before the first bullet, that are no sentence, are the location ("Tirana").
        const lead = raw.split(BULLET_SPLIT)[0].trim()
        if (lead !== '' && lead.split(/\s+/).length <= 6 && !/[.!?]$/.test(lead) && BULLET.test(raw)) raw = raw.slice(lead.length)
      }
    }
    if (raw === '') continue
    if (state.techNext) {
      technologies = raw
      state.techNext = false
      continue
    }
    const text = takeTech(raw)
    if (text === '') continue
    const parts = text.split(BULLET_SPLIT)
    const lead = (parts[0] ?? '').trim()
    const startsWithBullet = BULLET.test(text.trimStart().charAt(0))
    const pieces = startsWithBullet ? parts.slice(1) : parts.slice(1)
    if (!startsWithBullet && lead !== '') {
      const lastAt = bullets.length - 1
      if (!seenBullet) intro = intro === '' ? lead : `${intro} ${lead}`
      else if (lastAt >= 0 && !SENTENCE_END.test(bullets[lastAt]) && /^\p{Ll}/u.test(lead)) bullets[lastAt] = `${bullets[lastAt]} ${lead}`
      else stray.push(lead)
    }
    for (const piece of pieces) {
      const p = piece.trim()
      if (p === '') continue
      seenBullet = true
      bullets.push(p)
    }
  }
  // A bullet of three words or fewer, with no full stop, is a sidebar's fragment ("RabbitMQ", "AWS SNS", "Domain Driven Design").
  const real: string[] = []
  for (const b of bullets) {
    if (b.split(/\s+/).length <= 3 && !/[.!?]$/.test(b)) stray.push(b)
    else real.push(b)
  }
  return { intro: intro.trim(), bullets: real, stray, technologies: technologies.trim() }
}

// The text of a job as the import stores it: the introduction, one line for every bullet point, and the technologies.
export function jobDescription(job: Pick<CvJob, 'intro' | 'bullets' | 'technologies'>): string {
  const lines: string[] = []
  if (job.intro !== '') lines.push(job.intro)
  for (const b of job.bullets) lines.push(`- ${b}`)
  if (job.technologies !== '') lines.push(`Technologies: ${job.technologies}`)
  return lines.join('\n')
}

export function splitCv(text: string): SplitCv {
  const lines = text
    .split(/\r?\n/)
    .map((l) => l.trim())
    .filter((l) => l !== '')
  const header: string[] = []
  const sections: CvSection[] = []
  let current: CvSection | null = null
  for (const line of lines) {
    const kind = headingKind(line)
    if (kind !== null) {
      current = { kind, heading: line, lines: [] }
      sections.push(current)
    } else if (current) current.lines.push(line)
    else header.push(line)
  }
  const jobs: CvJob[] = []
  for (const section of sections) {
    if (section.kind !== 'experience') continue
    for (const block of jobBlocks(section.lines)) {
      const { company, title } = splitCompanyTitle(block.header)
      jobs.push({
        header: block.header,
        company,
        title,
        startDate: block.startDate,
        endDate: block.endDate,
        current: block.current,
        ...readBlock(block),
      })
    }
  }
  const usable = sections.some((s) => s.kind === 'profile' || s.kind === 'skills' || s.kind === 'experience' || s.kind === 'education')
  return { header, sections, jobs, usable }
}
