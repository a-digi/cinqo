// The tips shown at the top of the Career pages (steps 105 and 106): a few short slides per page that explain how
// the page is used. Plain data, no React, so it can be script-checked. A tip is a short title and a short text,
// written for a banner that is read at a glance, with no prices. It is editorial text, not configuration: it
// changes with what the app does, so it lives here and not in the pages. Every statement in it comes from what the
// page or the tool actually does. See plan/ai/tools/career/step-106-tips-on-every-career-page.md.
import { ACCENTS, type Accent } from '../../Components/JobsPage/jobsTheme'

// The dashboards' eight accent colours, by name.
export type TipTone = 'blue' | 'green' | 'amber' | 'rose' | 'violet' | 'teal' | 'sky' | 'orange'

export const TIP_TONES: readonly TipTone[] = ['blue', 'green', 'amber', 'rose', 'violet', 'teal', 'sky', 'orange']

export function toneAccent(tone: TipTone): Accent {
  return ACCENTS[TIP_TONES.indexOf(tone)]
}

// The icon on a tip's badge (the badge component maps each name to one of the app's icons).
export type TipIconName =
  | 'search'
  | 'match'
  | 'cv'
  | 'plus'
  | 'details'
  | 'job'
  | 'company'
  | 'people'
  | 'person'
  | 'layers'
  | 'target'
  | 'timeline'
  | 'camera'
  | 'upload'
  | 'monitor'
  | 'play'
  | 'stop'
  | 'log'
  | 'alert'
  | 'lock'
  | 'spark'
  | 'robot'
  | 'bolt'
  | 'clock'
  | 'check'
  | 'globe'
  | 'link'
  | 'pencil'

export const TIP_ICON_NAMES: readonly TipIconName[] = [
  'search',
  'match',
  'cv',
  'plus',
  'details',
  'job',
  'company',
  'people',
  'person',
  'layers',
  'target',
  'timeline',
  'camera',
  'upload',
  'monitor',
  'play',
  'stop',
  'log',
  'alert',
  'lock',
  'spark',
  'robot',
  'bolt',
  'clock',
  'check',
  'globe',
  'link',
  'pencil',
]

export interface PageTip {
  id: string
  // The dot's tooltip and what a screen reader hears.
  label: string
  title: string
  text: string
  tone: TipTone
  icon: TipIconName
  // A tip can point at Settings.
  settingsLink?: boolean
}

// The most words a tip's text may have: a tip is read at a glance.
export const MAX_TIP_WORDS = 30

export function wordCount(text: string): number {
  const words = text.trim().split(/\s+/)
  return words.length === 1 && words[0] === '' ? 0 : words.length
}

export type PageTipsId =
  | 'jobs'
  | 'cvs'
  | 'crawlMonitor'
  | 'portals'
  | 'companies'
  | 'recruiters'
  | 'profiles'
  | 'personas'
  | 'summary'
  | 'skills'
  | 'experience'
  | 'photo'
  | 'importCv'
  | 'settings'

export interface PageTipSet {
  // The page's name, for the carousel's accessible name ("Tips: Saved Jobs").
  name: string
  tips: PageTip[]
}

export const PAGE_TIPS: Record<PageTipsId, PageTipSet> = {
  jobs: {
    name: 'Saved Jobs',
    tips: [
      {
        id: 'search',
        label: 'Find jobs',
        title: 'Find jobs as you type',
        tone: 'blue',
        icon: 'search',
        text: 'Type in the search box and the list filters as you type. Narrow it further by location, company or platform; the Platform pill opens that portal’s page.',
      },
      {
        id: 'match',
        label: 'Job Match',
        title: 'Match a job with a profile',
        tone: 'violet',
        icon: 'match',
        text: 'Open a job’s menu and pick Job Match to let the AI compare it with a profile of yours. The score and matched skills then show on the job.',
      },
      {
        id: 'cv',
        label: 'A CV per job',
        title: 'A CV for each job',
        tone: 'green',
        icon: 'cv',
        text: 'From a job’s menu, let the AI generate a CV, build one yourself, or attach one you already made. Download it from the same menu.',
      },
      {
        id: 'add',
        label: 'Add your own',
        title: 'Add jobs you found yourself',
        tone: 'amber',
        icon: 'plus',
        text: 'Use Add job for a posting you found yourself. Such jobs are marked as yours; one without a link to the original simply has nothing to open.',
      },
      {
        id: 'details',
        label: 'Details',
        title: 'Open the full details',
        tone: 'sky',
        icon: 'details',
        text: 'View details shows the full text once a job has been crawled. Link a job to a company, and it also appears on that company’s page.',
      },
    ],
  },
  cvs: {
    name: 'CV Documents',
    tips: [
      {
        id: 'by-persona',
        label: 'By persona',
        title: 'One list per persona',
        tone: 'blue',
        icon: 'person',
        text: 'Pick a persona to see its CVs as cards. Click a card to edit it; hover it for preview, rename and delete.',
      },
      {
        id: 'builder',
        label: 'Builder',
        title: 'Build a CV in five steps',
        tone: 'violet',
        icon: 'layers',
        text: 'New CV opens a builder: persona and template, personal details, skills, experience, then title and review. The PDF is made from the persona’s own data.',
      },
      {
        id: 'photo',
        label: 'Photo',
        title: 'Your photo on the CV',
        tone: 'rose',
        icon: 'camera',
        text: 'The CV templates use the profile’s default photo. With several photos, you choose one when you create a CV. Add and manage photos on the Photo page.',
      },
      {
        id: 'attach',
        label: 'Attach to a job',
        title: 'Attach a CV to a job',
        tone: 'green',
        icon: 'job',
        text: 'On the Jobs page you can attach a CV from this list to a job. Removing it from the job never deletes the CV itself.',
      },
    ],
  },
  crawlMonitor: {
    name: 'Crawl Monitor',
    tips: [
      {
        id: 'live',
        label: 'Live view',
        title: 'Everything running right now',
        tone: 'teal',
        icon: 'monitor',
        text: 'Every portal link being crawled shows here and refreshes by itself. Listing crawls find the jobs; job detail crawls fetch each job’s full text.',
      },
      {
        id: 'auto',
        label: 'Auto-started',
        title: 'Started by itself',
        tone: 'violet',
        icon: 'bolt',
        text: 'A crawl marked as started automatically came from auto-discovery, not from a click. Switch auto-discovery on or off on the Portals page.',
      },
      {
        id: 'needs-you',
        label: 'Needs you',
        title: 'Sometimes it waits for you',
        tone: 'amber',
        icon: 'alert',
        text: 'If a site shows a Cloudflare check, the crawl waits for you to solve it in the browser window. The Needs you figure counts those.',
      },
      {
        id: 'log-stop',
        label: 'Log and stop',
        title: 'Read the log, or stop it',
        tone: 'sky',
        icon: 'log',
        text: 'Open a run’s log to see every step it took. Stop really interrupts the crawl on the server, not just this view.',
      },
    ],
  },
  portals: {
    name: 'Portals',
    tips: [
      {
        id: 'your-ai',
        label: 'Your AI',
        title: 'Your choice of AI',
        tone: 'violet',
        icon: 'robot',
        settingsLink: true,
        text: 'A portal or a link can use its own AI platform and model (the AI chip). With none set, the portal’s is used, then the general one in Settings.',
      },
      {
        id: 'cheaper-models',
        label: 'Cheaper models',
        title: 'Cheaper models can do it',
        tone: 'green',
        icon: 'spark',
        text: 'Low-cost models from OpenRouter can often write crawl instructions too. Pick a specific cheap model: the random free router often fails at tool use.',
      },
      {
        id: 'generate-once',
        label: 'Generate once',
        title: 'Generate once, crawl many times',
        tone: 'blue',
        icon: 'play',
        text: 'Generate lets the AI write a link’s crawl instructions. Crawl now then runs them without AI, on the server, and keeps going even if you leave the page.',
      },
      {
        id: 'reuse',
        label: 'Reuse',
        title: 'New links try what already works',
        tone: 'teal',
        icon: 'link',
        text: 'Adding a link to a portal that has instructions tries them first, with no AI. If they work on the new page, the link simply reuses them.',
      },
      {
        id: 'not-every-page',
        label: 'Not every page',
        title: 'Not every page can be crawled',
        tone: 'amber',
        icon: 'alert',
        text: 'Some sites block automated visits or load their jobs only after a click or scroll. After 3 failed tries a link shows “Probably can’t be crawled”.',
      },
      {
        id: 'paywalls',
        label: 'Paywalls',
        title: 'Paywalls and logins',
        tone: 'rose',
        icon: 'lock',
        text: 'Pages behind a paywall or a login cannot be crawled. Use the site’s public job list instead.',
      },
    ],
  },
  companies: {
    name: 'Companies',
    tips: [
      {
        id: 'directory',
        label: 'A directory',
        title: 'A directory of companies',
        tone: 'blue',
        icon: 'company',
        text: 'Companies are kept apart from the free-text name on each crawled job. Link a job to a company from the Jobs page and it shows up here.',
      },
      {
        id: 'open',
        label: 'Open a company',
        title: 'See a company’s jobs',
        tone: 'sky',
        icon: 'details',
        text: 'View opens a company’s page with the jobs linked to it. The company name in the Jobs list links straight to it, too.',
      },
      {
        id: 'safe',
        label: 'Safe to delete',
        title: 'Deleting never deletes jobs',
        tone: 'green',
        icon: 'check',
        text: 'Deleting a company only un-links its jobs; the jobs themselves are never deleted.',
      },
      {
        id: 'recruiters',
        label: 'Recruiters',
        title: 'Recruiters belong to companies',
        tone: 'violet',
        icon: 'people',
        text: 'Each recruiter is tied to a company. Add them on the Recruiters page; the Overview counts them.',
      },
    ],
  },
  recruiters: {
    name: 'Recruiters',
    tips: [
      {
        id: 'tied',
        label: 'Tied to a company',
        title: 'Every recruiter has a company',
        tone: 'blue',
        icon: 'company',
        text: 'A recruiter is tied to an existing company, so add the company first. Companies are listed on the Companies page.',
      },
      {
        id: 'fixed',
        label: 'Cannot change',
        title: 'The company cannot be changed',
        tone: 'amber',
        icon: 'alert',
        text: 'Once a recruiter is created, its company cannot be changed. To move one, delete it and create it again.',
      },
      {
        id: 'filter',
        label: 'Filter',
        title: 'See one company’s recruiters',
        tone: 'sky',
        icon: 'search',
        text: 'Use the company filter to see only one company’s recruiters. The Overview shows how many have an email address.',
      },
    ],
  },
  profiles: {
    name: 'Profiles',
    tips: [
      {
        id: 'start',
        label: 'Start here',
        title: 'One profile per job seeker',
        tone: 'blue',
        icon: 'person',
        text: 'Personas, skills and experience all hang off a profile. Create the profile first, then add a persona for each role.',
      },
      {
        id: 'completeness',
        label: 'Completeness',
        title: 'See how complete a profile is',
        tone: 'green',
        icon: 'check',
        text: 'The bar on each card shows how complete the profile is and what is missing. A link, a photo, a persona, skills and experience all count.',
      },
      {
        id: 'links',
        label: 'Links',
        title: 'Add your platforms',
        tone: 'violet',
        icon: 'link',
        text: '+ Platform adds a link such as LinkedIn. A link is the biggest single part of completeness, but the profile still needs the rest.',
      },
      {
        id: 'careful',
        label: 'Careful',
        title: 'Deleting is permanent',
        tone: 'rose',
        icon: 'alert',
        text: 'Deleting a profile permanently deletes its personas and everything they own, with no further confirmation.',
      },
    ],
  },
  personas: {
    name: 'Personas',
    tips: [
      {
        id: 'per-role',
        label: 'One per role',
        title: 'One persona per role',
        tone: 'blue',
        icon: 'target',
        text: 'A persona is one role you are pursuing, such as “Backend Engineer”. Each has its own summary, skills and experience.',
      },
      {
        id: 'why',
        label: 'Why',
        title: 'A clear picture for the AI',
        tone: 'violet',
        icon: 'spark',
        text: 'The AI reads a persona when matching jobs and writing CVs, so a persona focused on one role gives it a clearer picture.',
      },
      {
        id: 'next',
        label: 'Next',
        title: 'Then fill it in',
        tone: 'green',
        icon: 'layers',
        text: 'After creating a persona, add its Summary & Goals, Skills and Experience from the My Profile menu.',
      },
      {
        id: 'with-ai',
        label: 'With AI',
        title: 'Create personas with AI',
        tone: 'teal',
        icon: 'spark',
        text: 'Create with AI proposes up to three personas from what you have filled in. You review and edit them first. The chats stay under AI history.',
      },
    ],
  },
  summary: {
    name: 'Summary & Goals',
    tips: [
      {
        id: 'positioning',
        label: 'Positioning',
        title: 'How the persona presents itself',
        tone: 'blue',
        icon: 'pencil',
        text: 'Headline, summary and location describe the persona. The AI reads them when matching jobs and writing CVs.',
      },
      {
        id: 'goals',
        label: 'Goals',
        title: 'What you are looking for',
        tone: 'green',
        icon: 'target',
        text: 'Desired titles, desired locations and a minimum salary say what you want from a job.',
      },
      {
        id: 'completeness',
        label: 'Completeness',
        title: 'Headline and summary count',
        tone: 'amber',
        icon: 'check',
        text: 'A persona with both a headline and a summary adds to the profile’s completeness on the Profiles page.',
      },
      {
        id: 'not-secret',
        label: 'Not secret',
        title: 'Nothing here is secret',
        tone: 'rose',
        icon: 'lock',
        text: 'The AI reads this text when it writes CVs, so write it the way you would on a CV.',
      },
    ],
  },
  skills: {
    name: 'Skills',
    tips: [
      {
        id: 'per-persona',
        label: 'Per persona',
        title: 'Skills belong to a persona',
        tone: 'blue',
        icon: 'layers',
        text: 'Skills are kept per persona. The AI reads them when matching you against job postings.',
      },
      {
        id: 'add',
        label: 'Add and edit',
        title: 'Add as many as you need',
        tone: 'green',
        icon: 'plus',
        text: 'Type a skill and add it. Each skill can be edited afterwards.',
      },
      {
        id: 'completeness',
        label: 'Completeness',
        title: 'Three skills count',
        tone: 'amber',
        icon: 'check',
        text: 'Three skills on one persona make the Skills part of the profile’s completeness count.',
      },
    ],
  },
  experience: {
    name: 'Experience',
    tips: [
      {
        id: 'history',
        label: 'Work history',
        title: 'Your work history',
        tone: 'blue',
        icon: 'timeline',
        text: 'Experience entries are kept per persona. The AI reads them when matching you against job postings.',
      },
      {
        id: 'current',
        label: 'Current role',
        title: 'No end date means current',
        tone: 'green',
        icon: 'clock',
        text: 'An entry without an end date counts as your current role.',
      },
      {
        id: 'completeness',
        label: 'Completeness',
        title: 'One entry counts',
        tone: 'amber',
        icon: 'check',
        text: 'One experience entry on a persona makes the Experience part of the profile’s completeness count.',
      },
    ],
  },
  photo: {
    name: 'Photo',
    tips: [
      {
        id: 'one',
        label: 'One photo',
        title: 'Several photos, one default',
        tone: 'blue',
        icon: 'camera',
        text: 'A profile can have several photos. The default is used on CVs unless you pick another in the CV builder. One photo can be shared by many profiles.',
      },
      {
        id: 'cv',
        label: 'On your CV',
        title: 'Used by the CV templates',
        tone: 'green',
        icon: 'cv',
        text: 'The CV templates use this photo, so add it before you build a CV.',
      },
      {
        id: 'completeness',
        label: 'Completeness',
        title: 'A photo counts',
        tone: 'amber',
        icon: 'check',
        text: 'A photo is part of how complete a profile is on the Profiles page.',
      },
    ],
  },
  importCv: {
    name: 'Import from CV',
    tips: [
      {
        id: 'upload',
        label: 'Upload',
        title: 'Let the AI read your CV',
        tone: 'blue',
        icon: 'upload',
        text: 'Upload a CV and the AI proposes a profile, a persona, a summary, skills and experience from it.',
      },
      {
        id: 'review',
        label: 'You decide',
        title: 'You review everything',
        tone: 'green',
        icon: 'check',
        text: 'Nothing is saved until you have reviewed it. The AI’s proposal is only a draft.',
      },
      {
        id: 'existing',
        label: 'Existing profile',
        title: 'Add to a profile you have',
        tone: 'violet',
        icon: 'person',
        text: 'Instead of creating a new profile, you can attach the result to an existing one.',
      },
      {
        id: 'ai',
        label: 'AI choice',
        title: 'Uses the AI from Settings',
        tone: 'amber',
        icon: 'robot',
        settingsLink: true,
        text: 'Import uses the AI platform and model chosen in Settings. Open Settings to change it.',
      },
    ],
  },
  settings: {
    name: 'Settings',
    tips: [
      {
        id: 'one-choice',
        label: 'One choice',
        title: 'One AI choice for the whole tool',
        tone: 'violet',
        icon: 'robot',
        text: 'The platform and model chosen here are used for crawling, Job Match, CV generation and Import from CV.',
      },
      {
        id: 'overrides',
        label: 'Overrides',
        title: 'A portal or link can differ',
        tone: 'blue',
        icon: 'globe',
        text: 'A portal, or a single link, can use its own platform and model. This choice applies wherever none is set.',
      },
      {
        id: 'automatic',
        label: 'Automatic',
        title: 'Nothing saved is fine',
        tone: 'green',
        icon: 'spark',
        text: 'With nothing saved, the first platform that has an API key is used.',
      },
      {
        id: 'keys',
        label: 'Keys',
        title: 'Keys live in Cinqo',
        tone: 'rose',
        icon: 'lock',
        text: 'API keys are managed in Cinqo under the AI menu, not on this page.',
      },
    ],
  },
}
