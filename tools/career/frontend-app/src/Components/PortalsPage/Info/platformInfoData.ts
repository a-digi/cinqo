// The words of the "How platforms work" infographic (plan 262). The only place with its text: the poster, the empty-state teaser and the
// left-column card read from here. Every statement was checked against the code when it was written (the button and pill names are the ones on
// the page: PortalsPage, LinkStatusPills, instructionStats, the Crawler folder; the rules of saving jobs: jobs.source_url is unique; deleting a
// platform removes its links, a job only loses its link). No React and no layout here, so the rules and the words can be script-checked.
import { CRAWL_MONITOR_PATH, CRAWLER_SETTINGS_PATH, JOBS_PATH, SETTINGS_PATH } from '../../../paths'

export type ChapterId = 'overview' | 'platform' | 'links' | 'crawler' | 'instructions' | 'first-steps' | 'trouble'

// A step of the journey the progress strip can tell about from what the page has loaded; null = the page cannot tell.
export type JourneyKey = 'platform' | 'link' | 'instructions' | 'crawled'

export interface Caption {
  title: string
  text: string
}

export interface InfoLink {
  label: string
  path: string
}

export interface Chapter {
  id: ChapterId
  // The short name on the chapter bar.
  nav: string
  title: string
  // One sentence: what the chapter shows.
  promise: string
  captions: Caption[]
  // The real buttons to click, exactly as they are named on the page.
  click: string[]
  links: InfoLink[]
}

// The big picture at the top: five nodes of one flow. `chapter` is the chapter that zooms into the node (none for the last two).
export interface HeroNode {
  id: string
  label: string
  words: string
  chapter: ChapterId | null
}

export const HERO_NODES: HeroNode[] = [
  { id: 'platform', label: 'Platform', words: 'A job site you watch', chapter: 'platform' },
  { id: 'links', label: 'Links', words: 'The pages with job lists', chapter: 'links' },
  { id: 'crawler', label: 'Crawler', words: 'Reads them for you', chapter: 'crawler' },
  { id: 'jobs', label: 'Jobs', words: 'Saved once, in your list', chapter: null },
  { id: 'matches', label: 'Matches', words: 'Scored against your profiles', chapter: null },
]

export const HERO_TITLE = 'From a job site to jobs in your list'
export const HERO_LEAD =
  'A platform is a job site you want to keep an eye on. You tell Career which of its pages list jobs, the crawler reads those pages for you, and the jobs land in your Jobs list, already scored against your profiles.'

export const CHAPTERS: Chapter[] = [
  {
    id: 'overview',
    nav: 'The big picture',
    title: 'The big picture',
    promise: 'Five things, one after the other: you set up the first three once, then the jobs keep coming.',
    captions: [
      {
        title: 'You set up three things',
        text: 'A platform, a link to one of its job pages, and the instructions that say how to read that page.',
      },
      { title: 'The crawler does the reading', text: 'It opens the page, finds the postings, turns the page and saves what is new.' },
      { title: 'You work with the jobs', text: 'They appear on the Jobs page, scored against your profiles. Nothing else to maintain.' },
    ],
    click: [],
    links: [{ label: 'Open Jobs', path: JOBS_PATH }],
  },
  {
    id: 'platform',
    nav: 'Platform',
    title: 'A platform is a job site you watch',
    promise: 'One platform per site: a job board, or the career page of a company.',
    captions: [
      { title: 'It only groups links', text: 'A platform has a name and a list of links. It does not read anything by itself.' },
      {
        title: 'One per site',
        text: 'Create one for a job board or for "Acme careers", once, and put every page you want from that site under it.',
      },
      {
        title: 'Deleting it deletes its links',
        text: 'Deleting a platform permanently deletes every link it owns. The jobs already saved stay in your list.',
      },
      {
        title: 'The Overview',
        text: 'The ring on the left shows how many of your links are ready to crawl, and how many never saved a job.',
      },
    ],
    click: ['New platform'],
    links: [],
  },
  {
    id: 'links',
    nav: 'Links',
    title: 'Links are the pages with job lists',
    promise: 'A link is one page of the site that lists jobs: a search result, a category, a region.',
    captions: [
      {
        title: 'A page with a list, not the home page',
        text: 'Paste the address of a results page, for example "Backend jobs in Hamburg". The crawler reads the list on that page and the pages after it.',
      },
      {
        title: 'As many as you like',
        text: 'Add one link per search, region or category. Each link is crawled on its own and shows its own status.',
      },
      {
        title: 'A site that needs a login',
        text: 'Tick Requires login on the link. Crawl now then opens a window: you log in on the site itself and press "I am logged in". Links that need a login are never crawled automatically.',
      },
      {
        title: 'Its own AI, its own engine',
        text: 'A link can use its own AI platform and model (the AI chip) and its own way of reading pages. With none set, the platform’s AI is used, then the general one in Settings.',
      },
    ],
    click: ['Add link'],
    links: [{ label: 'Open Settings → AI', path: SETTINGS_PATH }],
  },
  {
    id: 'crawler',
    nav: 'Crawler',
    title: 'The crawler reads the pages for you',
    promise: 'It follows the same steps a person would, only faster and without getting tired.',
    captions: [
      {
        title: 'Open, find, turn the page, repeat',
        text: 'It opens the link’s page, finds the job postings, goes to the next page and repeats, then saves what it found.',
      },
      {
        title: 'Every job is saved once',
        text: 'A job is told apart by its web address, so crawling a link again only adds the jobs that are new.',
      },
      {
        title: 'It runs on the server',
        text: 'Crawl now keeps going even if you leave the page. The Crawl Monitor lists every crawl that is running right now.',
      },
      {
        title: 'On a schedule',
        text: 'Auto-discovery, at the top of the left column, crawls every link that is ready ("Crawl every …"). It never crawls a link that needs a login.',
      },
      {
        title: 'Filtered before it is saved',
        text: 'Ignore jobs (Settings → Job crawler) checks each new job against your rules first. It is off until you switch it on, and it never removes jobs that are already saved.',
      },
      { title: 'Then the jobs are yours', text: 'They show up on the Jobs page with a match score for each of your profiles.' },
    ],
    click: ['Crawl now'],
    links: [
      { label: 'Open the Crawl Monitor', path: CRAWL_MONITOR_PATH },
      { label: 'Open Settings → Job crawler', path: CRAWLER_SETTINGS_PATH },
    ],
  },
  {
    id: 'instructions',
    nav: 'Instructions',
    title: 'Instructions say how to read one site',
    promise: 'Every site is built differently. Instructions tell the crawler where things are on it.',
    captions: [
      {
        title: 'Two parts',
        text: 'The job list: where the postings are on the page and how to turn to the next page. The job details: where the title, company, place, date and description are on one posting. A link is "Instructed" only when it has both.',
      },
      { title: 'Written once, used every time', text: 'Once they are saved, Crawl now uses them without any AI, on the server.' },
      {
        title: 'The AI writes them',
        text: 'Generate with AI studies the page and writes the instructions for you. You can open them and change them yourself.',
      },
      {
        title: 'A new link may need nothing',
        text: 'Adding a link to a platform that already has instructions tries them first, with no AI. If they work on the new page, the link simply reuses them.',
      },
      {
        title: 'A better model writes better instructions',
        text: 'Some models fail or write poor instructions; premium models do a better job. The AI used is the link’s own, else the platform’s, else the one chosen in Settings.',
      },
    ],
    click: ['Generate with AI'],
    links: [{ label: 'Open Settings → AI', path: SETTINGS_PATH }],
  },
  {
    id: 'first-steps',
    nav: 'Your first platform',
    title: 'Your first platform in six steps',
    promise: 'About ten minutes, and most of it is the AI working.',
    captions: [],
    click: [],
    links: [],
  },
  {
    id: 'trouble',
    nav: 'When it fails',
    title: 'When it does not work',
    promise: 'Each message on a link says what is missing and what to do about it.',
    captions: [
      {
        title: 'Not every page can be crawled',
        text: 'Some sites block automated visits or only load their jobs after a click or a scroll. Pages behind a paywall cannot be crawled at all.',
      },
      {
        title: 'The log shows what happened',
        text: 'The Crawl Monitor and a run’s log list every step the crawl took, so you can see where it stopped.',
      },
    ],
    click: [],
    links: [{ label: 'Open the Crawl Monitor', path: CRAWL_MONITOR_PATH }],
  },
]

// The two ways Career can read a page, side by side (chapter "Instructions"). The Browser tool's engine decides which one reads a link; a link
// can ask for Rod for itself.
export interface EngineCard {
  id: 'katana' | 'rod'
  name: string
  tagline: string
  points: string[]
}

export const ENGINE_CARDS: EngineCard[] = [
  {
    id: 'katana',
    name: 'Katana',
    tagline: 'A rule, plus data picked on a real job',
    points: [
      'The job list is read with a rule: which links on the page are job postings.',
      'The job details are picked on a real, already crawled job: title, company, place, date, description.',
      'Generate with AI does both for the whole link in one go.',
    ],
  },
  {
    id: 'rod',
    name: 'Rod',
    tagline: 'AI-written instructions',
    points: [
      'The AI writes the instructions as text: where the jobs and each of their parts are on the page.',
      'One set for the job list, one for the job details; you can read and edit them.',
      'Chosen for every link on the Browser & Engine page, or for a single link on its own.',
    ],
  },
]
export const ENGINES_NOTE =
  'Which one is in use is set on the Browser & Engine page. You can leave it as it is: both are written by the same Generate with AI button.'

// The crawler's steps in the order it takes them (the picture of chapter "Crawler").
export const CRAWL_STEPS: { id: string; label: string; words: string }[] = [
  { id: 'open', label: 'Open the page', words: 'The link’s address' },
  { id: 'find', label: 'Find the postings', words: 'With the list instructions' },
  { id: 'turn', label: 'Turn the page', words: 'Next, next, next …' },
  { id: 'save', label: 'Save the new jobs', words: 'Each job once' },
  { id: 'details', label: 'Read each job', words: 'With the details instructions' },
]

// The three parts of a link the picture of chapter "Links" labels.
export const LINK_PARTS: { id: string; label: string; words: string }[] = [
  { id: 'title', label: 'Title', words: 'Your name for the page' },
  { id: 'url', label: 'Address', words: 'The results page' },
  { id: 'status', label: 'Status', words: 'Instructed, crawled …' },
  { id: 'ai', label: 'AI chip', words: 'Its own AI, if any' },
  { id: 'login', label: 'Login', words: 'Needs you to sign in' },
]

// The short part for a page with no platform yet (the teaser) and the card in the left column.
export const TEASER_TITLE = 'What are platforms for?'
export const TEASER_LEAD = 'A platform is a job site you watch. Career reads its job pages for you and fills your Jobs list.'
export const TEASER_STEPS: { id: string; title: string; text: string }[] = [
  { id: 'platform', title: 'Add a platform', text: 'A job site, like a job board.' },
  { id: 'link', title: 'Add a link', text: 'One of its search pages.' },
  { id: 'ai', title: 'Let the AI read it', text: 'It writes the instructions.' },
]
export const INFO_BUTTON = 'See how platforms work'
export const HEADER_BUTTON = 'How it works'
export const CARD_TITLE = 'How platforms work'
export const CARD_BUTTON = 'See how it works'
export const CARD_TEXT = 'Platform, links, crawler and instructions, in one picture.'
export const MODAL_TITLE = 'How platforms work'

// The steps of "Your first platform". `track` ties a step to what the page can tell (see platformInfoRules.journeyState); null = it cannot.
export interface FirstStep {
  id: string
  title: string
  text: string
  click: string
  track: JourneyKey | null
}

export const FIRST_STEPS: FirstStep[] = [
  {
    id: 'ai',
    title: 'Choose the AI',
    text: 'Crawling, Job Match and CVs all use the AI platform and model chosen in Settings.',
    click: 'Settings → AI',
    track: null,
  },
  { id: 'platform', title: 'Add a platform', text: 'Name the job site.', click: 'New platform', track: 'platform' },
  { id: 'link', title: 'Add a link', text: 'Paste the address of one of its search-results pages.', click: 'Add link', track: 'link' },
  {
    id: 'instructions',
    title: 'Let the AI write the instructions',
    text: 'It studies the page and works out how to read the job list and each job.',
    click: 'Generate with AI',
    track: 'instructions',
  },
  {
    id: 'crawl',
    title: 'Crawl',
    text: 'Fetch the jobs now, and watch the crawl in the Crawl Monitor.',
    click: 'Crawl now',
    track: 'crawled',
  },
  { id: 'jobs', title: 'See your jobs', text: 'They are on the Jobs page, scored against your profiles.', click: 'Jobs', track: null },
]

// What each message on a link means and what to do (chapter "When it fails").
export interface TroubleRow {
  id: string
  pill: string
  meaning: string
  todo: string
}

export const TROUBLE: TroubleRow[] = [
  {
    id: 'not-instructed',
    pill: 'Not instructed',
    meaning: 'There are no instructions for this link yet, so it cannot be crawled.',
    todo: 'Press Generate with AI on the link.',
  },
  {
    id: 'details-missing',
    pill: 'Job details missing',
    meaning: 'The job list can be crawled, but the jobs cannot be read in full yet.',
    todo: 'Generate the job details instructions of the link.',
  },
  {
    id: 'not-crawlable',
    pill: 'Not crawlable',
    meaning: 'The link has no instructions, so nothing can be crawled from it.',
    todo: 'Generate them with the AI, or add the jobs by hand.',
  },
  {
    id: 'probably-not',
    pill: 'Probably can’t be crawled',
    meaning: 'The AI could not write the job list instructions 3 times in a row.',
    todo: 'Try a premium model in Settings → AI. If it still fails, the site probably blocks robots or loads its jobs after a click: add the jobs by hand.',
  },
  {
    id: 'zero-jobs',
    pill: 'Crawled 0 jobs',
    meaning: 'The crawl ran but found no postings.',
    todo: 'Open the page to check it really lists jobs; if the page changed, generate the instructions again.',
  },
  {
    id: 'question',
    pill: 'Question for you',
    meaning: 'The AI is waiting for your answer: does this page list jobs?',
    todo: 'Open the link and answer it.',
  },
  {
    id: 'login',
    pill: 'Login needed',
    meaning: 'The site shows its jobs only to signed-in people.',
    todo: 'Tick Requires login, press Crawl now, log in in the window, then press "I am logged in".',
  },
]
