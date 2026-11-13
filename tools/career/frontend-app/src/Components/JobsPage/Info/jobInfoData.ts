// The words of the "How jobs work" infographic (plan 269), in the showcase look. The only place with its text: the modal, the summary that replaces the empty Jobs page, the header button
// and the side card read from here. Every statement was checked against the code when it was written, and the drift-guard tests read that code again (the labels of the page's buttons, the
// rules of Add job and of deleting, the status names, the limits). What is already said by the Platforms and the Profiles infographics is TAKEN from their data, never retyped, so the
// three stories cannot disagree. No React and no layout here, so the words and the rules can be script-checked.
import { CLEANUP_SETTINGS_PATH, GUIDE_JOBS_PATH, GUIDE_PATH, PLATFORMS_PATH } from '../../../paths'
import { HERO_NODES, CHAPTERS as PLATFORM_CHAPTERS } from '../../PortalsPage/Info/platformInfoData'
import { CHAPTERS as PROFILE_CHAPTERS } from '../../ProfilesPage/Info/profileInfoData'
import type { ShowcaseTone } from '../../../Shared/Infographic/showcase/showcaseTokens'

export type SectionId = 'way-in' | 'two-ways' | 'holds' | 'match' | 'details' | 'find' | 'application' | 'first-steps' | 'good'

// The icon of a four-up item and of a card: one of the tool's own icons (the picture component maps each key).
export type JobIcon =
  | 'title'
  | 'details'
  | 'match'
  | 'cv'
  | 'search'
  | 'board'
  | 'recruiter'
  | 'applied'
  | 'link'
  | 'clock'
  | 'lock'
  | 'limit'
  | 'trash'
  | 'copy'

export interface JobSection {
  id: SectionId
  // The short link in the local bar.
  nav: string
  tone: ShowcaseTone
  eyebrow: string
  title: string
  lead: string
}

// The bands alternate: the hero and the closing are black, so the first band is light.
export const SECTIONS: JobSection[] = [
  {
    id: 'way-in',
    nav: 'The way in',
    tone: 'light',
    eyebrow: 'The way in',
    title: 'A job starts as a link.',
    lead: 'Your Jobs list is filled from the pages you point Career to. No platform with a link, no crawled jobs: that is why this list is empty until you add one.',
  },
  {
    id: 'two-ways',
    nav: 'Two ways in',
    tone: 'dark',
    eyebrow: 'Two ways in',
    title: 'Crawled, or added by hand.',
    lead: 'A job gets here from a platform’s links, or because you put it here yourself. After that they are the same.',
  },
  {
    id: 'holds',
    nav: 'What a job holds',
    tone: 'light',
    eyebrow: 'What a job holds',
    title: 'More than a title.',
    lead: 'Each job is a small file: who offers it, what it says, how well it fits you, and the CV you made for it.',
  },
  {
    id: 'match',
    nav: 'Matched to you',
    tone: 'dark',
    eyebrow: 'The match',
    title: 'Every job, measured against every profile.',
    lead: 'A score tells you where to look first, and the rules of a profile can put the good ones on its job board for you.',
  },
  {
    id: 'details',
    nav: 'Details',
    tone: 'light',
    eyebrow: 'The details',
    title: 'The full posting is read second.',
    lead: 'The crawler first saves the job, then opens its own page to read the description. You can see which jobs still wait for it.',
  },
  {
    id: 'find',
    nav: 'Find the right one',
    tone: 'dark',
    eyebrow: 'Find the right one',
    title: 'Search it, sort it, narrow it down.',
    lead: 'The list is there to be cut down to what matters today.',
  },
  {
    id: 'application',
    nav: 'To an application',
    tone: 'light',
    eyebrow: 'From a job to an application',
    title: 'Put it on a board. Make the CV.',
    lead: 'A job you like moves on from this list, to the job board of the profile that wants it.',
  },
  {
    id: 'first-steps',
    nav: 'First jobs',
    tone: 'dark',
    eyebrow: 'Your first jobs',
    title: 'Four steps to a full list.',
    lead: 'This is what you do once. After that the crawler keeps the list fresh.',
  },
  {
    id: 'good',
    nav: 'Good to know',
    tone: 'light',
    eyebrow: 'Good to know',
    title: 'Before you start.',
    lead: 'The things that surprise people the first time.',
  },
]

// ---- the hero

export const HERO_TITLE = 'Jobs. Found for you.'
export const HERO_LEAD = 'Career reads the pages you point it to, and every posting lands here, already measured against your profiles.'
export const HERO_BUTTON = 'See how it works'
export const HERO_LINK = { label: 'Add a platform', path: PLATFORMS_PATH }
export const BAR_NAME = 'Jobs'
export const MODAL_TITLE = 'How jobs work'
export const CLOSING_LINE = 'Point it to the pages. The jobs come to you.'

// The three jobs of the hero's picture: a title, a platform and a match (the numbers are only an example).
export const HERO_CARDS: { title: string; platform: string; score: number }[] = [
  { title: 'Backend Engineer', platform: 'Acme Careers', score: 92 },
  { title: 'Team Lead', platform: 'Jobs.example', score: 81 },
  { title: 'Data Engineer', platform: 'By hand', score: 64 },
]

// ---- 1 the way in: the four steps of the pinned stage. The words are the Platforms infographic's own (its big-picture nodes and the promises of its chapters).
const platformPromise = (id: string): string => PLATFORM_CHAPTERS.find((c) => c.id === id)?.promise ?? ''
// A caption of the Platforms infographic, word for word (the login story is told there once).
const platformCaption = (chapter: string, title: string): string =>
  PLATFORM_CHAPTERS.find((c) => c.id === chapter)?.captions.find((c) => c.title === title)?.text ?? ''
export const WAY_IN_STEPS: { id: string; title: string; text: string }[] = [
  { id: 'platform', title: HERO_NODES[0].label, text: `${HERO_NODES[0].words}. ${platformPromise('platform')}` },
  { id: 'links', title: HERO_NODES[1].label, text: `${HERO_NODES[1].words}. ${platformPromise('links')}` },
  { id: 'crawler', title: HERO_NODES[2].label, text: `${HERO_NODES[2].words}. ${platformPromise('crawler')}` },
  { id: 'jobs', title: 'Your Jobs list', text: `${HERO_NODES[3].words}. Every new posting is added once, with the platform’s name on it.` },
]

// ---- 2 two ways in (the segmented control)
export interface WayPoint {
  title: string
  text: string
}
export const TWO_WAYS: { id: 'crawled' | 'by-hand'; label: string; tagline: string; points: string[] }[] = [
  {
    id: 'crawled',
    label: 'Crawled',
    tagline: 'From a platform’s links, again and again.',
    points: [
      'You set up a platform and a link once; the crawler then reads that page and saves what is new.',
      'Every row shows the name of the platform it came from.',
      'A job you delete is not added again by the crawler.',
    ],
  },
  {
    id: 'by-hand',
    label: 'Added by hand',
    tagline: 'Found on LinkedIn, in an email, from a friend.',
    points: [
      'Add job needs only a title; the company is picked from your companies.',
      'It gets everything a crawled job gets: Job Match, Generate CV and the company link.',
      'A job that already exists is refused, and you are pointed to it.',
    ],
  },
]

// ---- 3 what a job holds (the four-up row)
export const HOLDS: { id: string; icon: JobIcon; title: string; text: string }[] = [
  { id: 'title', icon: 'title', title: 'Title and company', text: 'The company can be linked to a company record.' },
  { id: 'details', icon: 'details', title: 'The details', text: 'The full description, read in a second pass.' },
  { id: 'match', icon: 'match', title: 'A match', text: 'A score for each profile.' },
  { id: 'cv', icon: 'cv', title: 'A CV', text: 'Made for this job, by the AI or by you.' },
]

// ---- 4 matched to you
export const MATCH_NUMBER = { value: '92', label: 'percent match, for one profile' }
const profileCaption = (title: string): { title: string; text: string } => {
  const jobs = PROFILE_CHAPTERS.find((c) => c.id === 'jobs')
  return jobs?.captions.find((c) => c.title === title) ?? { title, text: '' }
}
export const MATCH_CAPTIONS: { title: string; text: string }[] = [
  // the first and the last are the Profiles infographic's own sentences
  profileCaption('A match score for each profile'),
  {
    title: 'It keeps up by itself',
    text: 'A job without a match gets its best match from the manual engine, no AI, every few minutes. Match unmatched does it at once.',
  },
  { title: 'Or let the AI judge one', text: 'Job Match has an AI choice for a single job, and you can watch it work.' },
  profileCaption('Auto-add rules'),
]
export const NOT_JUDGED_NOTE = 'A job whose target role has no skills or desired titles yet cannot be judged: add some and it is matched.'

// ---- 5 details
export const DETAIL_STATES: { id: 'not-crawled' | 'failed' | 'crawled'; label: string; text: string }[] = [
  { id: 'not-crawled', label: 'Not crawled yet', text: 'The job is saved, its description has not been read.' },
  {
    id: 'failed',
    label: 'Failed to crawl',
    text: 'The page could not be read: a login, a limit, or a page the crawler does not understand.',
  },
  { id: 'crawled', label: 'View job details', text: 'The description is saved; open it from the row.' },
]
export const DETAIL_CAPTIONS: { title: string; text: string }[] = [
  {
    title: 'Crawl missing details',
    text: 'Reads the details of every job that has none, or whose last try failed. Its button shows how many.',
  },
  { title: 'Recrawl', text: 'Reads one job again, or all the jobs you ticked.' },
  { title: 'Open original posting', text: 'Goes to the job on the site it came from.' },
]

// ---- 6 find the right one
export const FIND_ITEMS: { id: string; title: string; text: string }[] = [
  { id: 'search', title: 'Search', text: 'Title, company or description.' },
  { id: 'sort', title: 'Sort', text: 'Newest first, or best match first.' },
  { id: 'filters', title: 'Filters', text: 'Any location, any company, any platform.' },
]
export const STAT_CARDS: { id: string; label: string; text: string }[] = [
  { id: 'total', label: 'All jobs', text: 'How many jobs are saved.' },
  { id: 'matched', label: 'Matched', text: 'How many have a match, and the average score.' },
  { id: 'platform', label: 'By platform', text: 'Where they came from.' },
]

// ---- 7 from a job to an application
export const APPLICATION_ITEMS: { id: string; icon: JobIcon; title: string; text: string }[] = [
  { id: 'board', icon: 'board', title: 'Add to job board', text: 'The job starts in Not Reviewed on each profile’s board you pick.' },
  { id: 'recruiter', icon: 'recruiter', title: 'Recruiter', text: 'Map a recruiter to the job.' },
  { id: 'cv', icon: 'cv', title: 'CV', text: 'Generate one with the AI, build it in the CV Builder, or attach one you made.' },
  {
    id: 'applied',
    icon: 'applied',
    title: 'Applied',
    text: 'Move it through the statuses: Applied, Declined, Stopped application, or your own.',
  },
]

// The columns of a profile's job board in the picture, in the order of the board (the server seeds Not Reviewed; Applied, Declined and Stopped application are the other fixed ones).
export const BOARD_COLUMNS: string[] = ['Not Reviewed', 'Applied', 'Declined', 'Stopped application']

// The profiles of the match picture (made-up names and scores) and the auto-add rule it shows.
export const SAMPLE_PROFILES: { name: string; score: number }[] = [
  { name: 'Ada', score: 92 },
  { name: 'Ben', score: 71 },
  { name: 'Cem', score: 38 },
]
export const RULE_EXAMPLE = { atLeast: 70, goesTo: 'the job board' }

// ---- 8 the first steps: the progress comes from what the page loaded (a platform exists, jobs have arrived, jobs are matched)
export interface FirstStepSpec {
  id: 'platform' | 'links' | 'jobs' | 'match'
  title: string
  text: string
  // The real button or page to use, as it is named on the page.
  click: string
}
export const FIRST_STEPS: FirstStepSpec[] = [
  { id: 'platform', title: 'Add a platform', text: 'A job site you want to watch.', click: 'Platforms → New platform' },
  {
    id: 'links',
    title: 'Add its links',
    text: 'The pages of the site that list jobs, and how to read them.',
    click: 'On the platform: Add link',
  },
  { id: 'jobs', title: 'Let it crawl, or add a job', text: 'The first postings land in this list.', click: 'Add job' },
  { id: 'match', title: 'Look at the matches', text: 'Sort by best match and open the first one.', click: 'Sort: Best match first' },
]

// ---- 9 good to know: six cards; a note number points to a footnote
export const GOOD_CARDS: { id: string; icon: JobIcon; title: string; text: string; note?: number }[] = [
  { id: 'link', icon: 'link', title: 'Crawling needs a link', text: 'A platform with no link has nothing to read.' },
  {
    id: 'time',
    icon: 'clock',
    title: 'It takes a while',
    text: 'A crawl takes minutes, not seconds; you can watch the running ones above the list.',
    note: 1,
  },
  { id: 'login', icon: 'lock', title: 'Some sites want a login', text: platformCaption('links', 'A site that needs a login'), note: 2 },
  {
    id: 'limit',
    icon: 'limit',
    title: 'Some sites say slow down',
    text: 'A site that answers “too many requests” (HTTP 429) is a rate limit: the crawler reports it and waits as the site asks before it tries again.',
  },
  {
    id: 'delete',
    icon: 'trash',
    title: 'Deleting has a price',
    text: 'A crawled job is not added again until you allow it under Job cleanup; a job you added by hand is gone for good; its card on a job board goes too.',
  },
  {
    id: 'dup',
    icon: 'copy',
    title: 'No doubles',
    text: 'A job with the same posting address, or the same title and company, is not added twice.',
  },
]
export const FOOTNOTES: string[] = [
  'The running crawls are shown in a strip above the list while they work.',
  'A link that needs a login is never crawled automatically; the Platforms infographic shows the steps.',
]
export const CLEANUP_LINK = { label: 'Job cleanup', path: CLEANUP_SETTINGS_PATH }

// ---- the entry points
export const TEASER_TITLE = 'No jobs yet'
export const TEASER_LEAD = 'Jobs come from the links of your platforms, or from you. See how it works, then set up the first platform.'
export const TEASER_NEXT: Record<'platform' | 'crawl', string> = {
  platform: 'Next: add a platform, a job site you want to watch.',
  crawl: 'You have a platform. Next: add its links and let it crawl, or add a job yourself.',
}
export const TEASER_BUTTONS = { info: 'See how jobs work', platform: 'Add a platform', platforms: 'Open Platforms', add: 'Add job' }
export const HEADER_BUTTON = 'How it works'
export const CARD_TITLE = 'How jobs work'
export const CARD_TEXT = 'Where jobs come from, how they are matched, and what to do with them.'
export const CARD_BUTTON = 'See how it works'
export const GUIDE_LINK = { label: 'Getting started', path: GUIDE_PATH }
export const GUIDE_JOBS_LINK = { label: 'Jobs guide', path: GUIDE_JOBS_PATH }
