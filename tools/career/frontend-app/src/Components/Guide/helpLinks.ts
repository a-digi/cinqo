// Which help a Career page opens (plan/ai/tools/career/step-239-qa-pages-as-links-in-their-pages.md, step 1). Pure: no React, so it can be
// script-checked. A page asks helpHref(its own path): the Questions about that page when there are some (the Questions page filtered with
// ?page=<key>, which also offers the page's guide), else the guide that covers it, else nothing.
import {
  APPLICATIONS_PATH,
  CLEANUP_SETTINGS_PATH,
  COMPANIES_PATH,
  CRAWLER_SETTINGS_PATH,
  CRAWL_MONITOR_PATH,
  CV_DOCUMENTS_PATH,
  EDUCATION_PATH,
  FAQ_PATH,
  GOALS_PATH,
  GUIDE_CONTACTS_PATH,
  GUIDE_JOBS_PATH,
  GUIDE_PROFILE_PATH,
  IMPORT_CV_PATH,
  JOBS_PATH,
  LANGUAGES_PATH,
  PERSONAS_PATH,
  PLATFORMS_PATH,
  PROFILES_PATH,
  PROFILE_IMAGE_PATH,
  RECRUITERS_PATH,
  SETTINGS_PATH,
} from '../../paths'
import { CAREER_PATH_PREFIX, pathOnly } from '../../Shared/Timeline/timelineRules'
import { KNOWN_PAGE_PATHS } from './guideRules'
import type { Question } from './guideTypes'

export interface HelpEntry {
  // The value of ?page= on the Questions page: the questions whose link goes to this page.
  key: string
  // The guide that covers the page, if one does.
  guide?: string
}

// The pages that have help of their own (exact address of the page).
export const HELP_PAGES: Readonly<Record<string, HelpEntry>> = {
  [PROFILES_PATH]: { key: 'profiles', guide: GUIDE_PROFILE_PATH },
  [PERSONAS_PATH]: { key: 'persona', guide: GUIDE_PROFILE_PATH },
  [GOALS_PATH]: { key: 'goals', guide: GUIDE_PROFILE_PATH },
  [EDUCATION_PATH]: { key: 'education', guide: GUIDE_PROFILE_PATH },
  [LANGUAGES_PATH]: { key: 'languages', guide: GUIDE_PROFILE_PATH },
  [PROFILE_IMAGE_PATH]: { key: 'photo', guide: GUIDE_PROFILE_PATH },
  [IMPORT_CV_PATH]: { key: 'import', guide: GUIDE_PROFILE_PATH },
  [CV_DOCUMENTS_PATH]: { key: 'cvs', guide: GUIDE_PROFILE_PATH },
  [JOBS_PATH]: { key: 'jobs', guide: GUIDE_JOBS_PATH },
  [APPLICATIONS_PATH]: { key: 'applications' },
  [PLATFORMS_PATH]: { key: 'platforms', guide: GUIDE_JOBS_PATH },
  [CRAWL_MONITOR_PATH]: { key: 'crawl-monitor', guide: GUIDE_JOBS_PATH },
  [SETTINGS_PATH]: { key: 'ai', guide: GUIDE_JOBS_PATH },
  [CLEANUP_SETTINGS_PATH]: { key: 'cleanup', guide: GUIDE_JOBS_PATH },
  [CRAWLER_SETTINGS_PATH]: { key: 'crawler', guide: GUIDE_JOBS_PATH },
  [COMPANIES_PATH]: { key: 'companies', guide: GUIDE_CONTACTS_PATH },
  [RECRUITERS_PATH]: { key: 'recruiters', guide: GUIDE_CONTACTS_PATH },
}

// Every other page falls back to the guide of its area (the longest matching prefix wins): the profile's own pages to "Profile & CV",
// the job pages to "Finding jobs", the company and recruiter pages to "Companies & recruiters".
const FALLBACK_GUIDES: readonly (readonly [string, string])[] = [
  ['/tools/career/profiles', GUIDE_PROFILE_PATH],
  ['/tools/career/personas', GUIDE_PROFILE_PATH],
  ['/tools/career/persona-details', GUIDE_PROFILE_PATH],
  ['/tools/career/profile-settings', GUIDE_PROFILE_PATH],
  ['/tools/career/profile-image', GUIDE_PROFILE_PATH],
  ['/tools/career/goals', GUIDE_PROFILE_PATH],
  ['/tools/career/skills', GUIDE_PROFILE_PATH],
  ['/tools/career/experience', GUIDE_PROFILE_PATH],
  ['/tools/career/education', GUIDE_PROFILE_PATH],
  ['/tools/career/languages', GUIDE_PROFILE_PATH],
  ['/tools/career/courses', GUIDE_PROFILE_PATH],
  ['/tools/career/hobbies', GUIDE_PROFILE_PATH],
  ['/tools/career/import-cv', GUIDE_PROFILE_PATH],
  ['/tools/career/cv-documents', GUIDE_PROFILE_PATH],
  ['/tools/career/cv-builder', GUIDE_PROFILE_PATH],
  ['/tools/career/jobs', GUIDE_JOBS_PATH],
  ['/tools/career/job-details', GUIDE_JOBS_PATH],
  ['/tools/career/job-board', GUIDE_JOBS_PATH],
  ['/tools/career/platforms', GUIDE_JOBS_PATH],
  ['/tools/career/settings', GUIDE_JOBS_PATH],
  ['/tools/career/companies', GUIDE_CONTACTS_PATH],
  ['/tools/career/recruiters', GUIDE_CONTACTS_PATH],
]

function entryFor(path: string, pages: Readonly<Record<string, HelpEntry>> = HELP_PAGES): HelpEntry | undefined {
  return Object.hasOwn(pages, path) ? pages[path] : undefined
}

function isUnder(path: string, prefix: string): boolean {
  return path === prefix || path.startsWith(`${prefix}/`)
}

// The guide that covers a page, or undefined (a page with its own entry, else the longest matching fallback prefix).
export function guideForPage(pagePath: string): string | undefined {
  const path = pathOnly(pagePath)
  const own = entryFor(path)
  if (own !== undefined) return own.guide
  let best: readonly [string, string] | undefined
  for (const entry of FALLBACK_GUIDES) {
    if (isUnder(path, entry[0]) && (best === undefined || entry[0].length > best[0].length)) best = entry
  }
  return best?.[1]
}

// The key of the page a question is about: the page its link goes to; undefined for a question without a link or with one to a page that
// has no entry in HELP_PAGES.
export function questionPageKey(question: Question): string | undefined {
  if (question.link === undefined) return undefined
  return entryFor(pathOnly(question.link.path))?.key
}

// The questions to show for ?page=<key>: the ones about that page. No key, or a key no page has, shows all of them.
export function questionsForPage(questions: readonly Question[], key: string | null): Question[] {
  if (key === null || key === '' || !Object.values(HELP_PAGES).some((entry) => entry.key === key)) return [...questions]
  return questions.filter((q) => questionPageKey(q) === key)
}

// The page the key stands for (its entry), or undefined for an unknown key.
export function helpEntryForKey(key: string | null): { path: string; entry: HelpEntry } | undefined {
  if (key === null) return undefined
  const found = Object.entries(HELP_PAGES).find(([, entry]) => entry.key === key)
  return found === undefined ? undefined : { path: found[0], entry: found[1] }
}

// Where a page's help link goes: the Questions about the page when there are some, else the guide that covers it, else null (no link).
export function helpHref(pagePath: string, questions: readonly Question[]): string | null {
  const path = pathOnly(pagePath)
  const key = entryFor(path)?.key
  if (key !== undefined && questions.some((q) => questionPageKey(q) === key)) return `${FAQ_PATH}?page=${key}`
  return guideForPage(path) ?? null
}

// Why the table is not fit, one message each (empty = fine): every page and guide is a page of paths.ts, keys are unique, and every
// question is about a page that has an entry (so it shows up under that page).
export function helpProblems(
  questions: readonly Question[],
  knownPaths: readonly string[] = KNOWN_PAGE_PATHS,
  pages: Readonly<Record<string, HelpEntry>> = HELP_PAGES,
): string[] {
  const problems: string[] = []
  const keys = new Set<string>()
  for (const [path, entry] of Object.entries(pages)) {
    if (!path.startsWith(CAREER_PATH_PREFIX) || !knownPaths.includes(path)) problems.push(`help page "${path}" is not a page of the tool`)
    if (entry.key.trim() === '') problems.push(`help page "${path}" has no key`)
    else if (keys.has(entry.key)) problems.push(`two help pages have the key "${entry.key}"`)
    else keys.add(entry.key)
    if (entry.guide !== undefined && !knownPaths.includes(entry.guide))
      problems.push(`help page "${path}" has an unknown guide "${entry.guide}"`)
  }
  for (const [prefix, guide] of FALLBACK_GUIDES) {
    if (!knownPaths.includes(guide)) problems.push(`fallback "${prefix}" has an unknown guide "${guide}"`)
  }
  for (const q of questions) {
    if (q.link !== undefined && entryFor(pathOnly(q.link.path), pages) === undefined) {
      problems.push(`question "${q.id}" is about "${q.link.path}", which has no help entry`)
    }
  }
  return problems
}
