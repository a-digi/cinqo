// The words of the "How profiles work" infographic (plan 266). The only place with its text: the poster, the empty-state teaser and the left-column card
// read from here. Every statement was checked against the code when it was written (completeness weights and bands: backend profile/completeness.go; the
// auto-add rule limits: profilerules.go; the button and tile names are the ones on the page; what a target role follows and what it chooses: the Guide
// "Profile & CV" and the persona pages). No React and no layout here, so the rules and the words can be script-checked.
import { CV_DOCUMENTS_PATH, GUIDE_PROFILE_PATH, IMPORT_CV_PATH, JOBS_PATH, PERSONAS_PATH, SETTINGS_PATH } from '../../../paths'
import type { Caption, Chapter as SharedChapter, InfoLink } from '../../../Shared/Infographic/infographicTypes'

export type ChapterId = 'overview' | 'profile' | 'roles' | 'data' | 'completeness' | 'jobs' | 'first-steps' | 'careful'

// A step of the journey the progress strip can tell about from what the page has loaded; null = the page cannot tell.
export type JourneyKey = 'profile' | 'links' | 'role' | 'content'

export type { Caption, InfoLink }
export type Chapter = SharedChapter<ChapterId>

// The big picture at the top: five nodes of one flow. `chapter` is the chapter that zooms into the node.
export interface HeroNode {
  id: string
  label: string
  words: string
  chapter: ChapterId
}

export const HERO_NODES: HeroNode[] = [
  { id: 'profile', label: 'Profile', words: 'One job seeker', chapter: 'profile' },
  { id: 'roles', label: 'Target roles', words: 'The kinds of job you want', chapter: 'roles' },
  { id: 'data', label: 'Your data', words: 'Skills, experience, education', chapter: 'data' },
  { id: 'matches', label: 'Job matches', words: 'Scored for each profile', chapter: 'jobs' },
  { id: 'cvs', label: 'CVs and applications', words: 'Made from a target role', chapter: 'jobs' },
]

export const HERO_TITLE = 'From who you are to the jobs and CVs that fit'
export const HERO_LEAD =
  'A profile is one job seeker. You describe the person once, add a target role for each kind of job they want, and the rest of the tool works from it: jobs are scored against the profile, CVs are made from a target role, and the job board and the applications follow.'

export const CHAPTERS: Chapter[] = [
  {
    id: 'overview',
    nav: 'The big picture',
    title: 'The big picture',
    promise: 'You set up the first three once; the rest then works from them.',
    captions: [
      { title: 'You describe yourself once', text: 'A profile holds who you are: your details, skills, experience, education and more.' },
      {
        title: 'Target roles say what you want',
        text: 'Each target role is one kind of job you are looking for, for example Backend Engineer.',
      },
      {
        title: 'The tool works from it',
        text: 'Jobs get a match score for each profile, CVs are made from a target role, and the job board and the applications follow.',
      },
    ],
    click: [],
    links: [{ label: 'Read the whole guide', path: GUIDE_PROFILE_PATH }],
  },
  {
    id: 'profile',
    nav: 'A profile',
    title: 'A profile is one job seeker',
    promise: 'One profile per person: it holds everything about them and their job search.',
    captions: [
      {
        title: 'One per person',
        text: 'Two job seekers, two profiles. Create one with New profile, or let the AI read a CV with Import CV.',
      },
      {
        title: 'Everything hangs off it',
        text: 'Target roles, skills, experience, education, languages, courses, hobbies, photos and platform links all belong to a profile.',
      },
      {
        title: 'Its card has a tile for every page',
        text: 'Open any page of the profile from its card: Target roles, Summary, Goals, Skills, Experience, Education, Languages, Courses, Hobbies, and Applications, which opens its job board.',
      },
      {
        title: 'Links and photos',
        text: '+ Platform adds a link such as LinkedIn. A profile can have several photos, one of them the default, and the CV designs use them.',
      },
    ],
    click: ['New profile', 'Import CV'],
    links: [],
  },
  {
    id: 'roles',
    nav: 'Target roles',
    title: 'Target roles are the kinds of job you want',
    promise: 'A target role is one kind of job the profile is pursuing, for example Backend Engineer.',
    captions: [
      {
        title: 'One role for each kind of job',
        text: 'Pick the profile on the Target roles page, then use New target role for each kind of job.',
      },
      { title: 'It follows the profile', text: 'A target role shows the profile’s summary and goals until you word its own.' },
      {
        title: 'It chooses what it uses',
        text: 'The profile keeps one library of skills and one of experience. Each target role chooses which ones it uses, can reword an experience for itself, or add entries of its own.',
      },
      { title: 'Always its own', text: 'A target role’s headline and location are always its own.' },
      { title: 'CVs come from a role', text: 'A CV is made from one target role, in one of a few fixed designs.' },
      { title: 'The AI reads them', text: 'The AI reads a target role’s data when it matches jobs and when it writes CVs.' },
    ],
    click: ['New target role'],
    links: [{ label: 'Open Target roles', path: PERSONAS_PATH }],
  },
  {
    id: 'data',
    nav: 'What goes in',
    title: 'What goes in a profile',
    promise: 'Some things are the profile’s and shared by every role; a target role chooses from the others.',
    captions: [
      {
        title: 'Summary and goals',
        text: 'Written once for the profile. Desired titles, desired locations, a minimum salary and remote work are its goals; a target role follows them unless it has its own.',
      },
      { title: 'Skills', text: 'One library of skills for the profile. The AI reads them when it matches you against job postings.' },
      {
        title: 'Experience',
        text: 'One library of work history. A target role chooses which entries it uses and can reword their title or description.',
      },
      { title: 'Education', text: 'Schools and degrees, shared by all target roles. An entry without an end date counts as ongoing.' },
      {
        title: 'Languages',
        text: 'The languages you speak or write, each with an optional level from C2 down to A1 (or Native), spoken, written or both.',
      },
      {
        title: 'Courses',
        text: 'Short trainings and certificates, with the provider, the completed date and a link to the certificate if you have one.',
      },
      { title: 'Hobbies', text: 'What you do outside work, one entry per hobby with an optional description.' },
      { title: 'Photos', text: 'Several photos, one of them the default. The CV designs use them.' },
    ],
    click: [],
    links: [],
  },
  {
    id: 'completeness',
    nav: 'Completeness',
    title: 'Completeness shows what is still missing',
    promise: 'Seven parts, 100 points in all: the bar on each card adds up what is done.',
    captions: [
      { title: 'The bar on each card', text: 'It shows the percentage and a word. What is missing is listed under it, heaviest first.' },
      { title: 'The four words', text: 'Below 40 is Just started, from 40 Getting there, from 70 Strong, and 100 is Complete.' },
      { title: 'A link counts most', text: 'Platform links are worth 25, the biggest single part, but the profile still needs the rest.' },
      {
        title: 'Some parts need a target role',
        text: 'Headline and summary need a target role that has both. Skills (at least three) and Experience (at least one) count in the profile’s library or in its best target role, whichever has more.',
      },
    ],
    click: [],
    links: [],
  },
  {
    id: 'jobs',
    nav: 'Jobs and CVs',
    title: 'From a profile to jobs and CVs',
    promise: 'Once the profile is filled in, the rest of the tool uses it.',
    captions: [
      {
        title: 'A match score for each profile',
        text: 'Every job gets a score against your profiles. The AI reads your target roles, skills and experience when it matches.',
      },
      {
        title: 'Auto-add rules',
        text: 'A profile’s Settings can add new jobs to its job board by score: a rule says that at least N% goes to a status. A profile can have up to 10 rules, the first rule that fits decides, and rules only act on jobs found from now on.',
      },
      {
        title: 'A job board and applications',
        text: 'Each profile has its own job board, and the number of its applications is shown on its card.',
      },
      { title: 'CVs', text: 'Create a CV from a target role; Create CV with AI lets the AI choose the design.' },
      { title: 'Import a CV', text: 'Import CV reads a CV into a profile. You review everything before it is saved.' },
      {
        title: 'Let the AI improve a text',
        text: 'The sparkle icon on a card lets the AI suggest better texts. Nothing changes until you approve it, and a declined suggestion is deleted.',
      },
    ],
    click: ['Import CV'],
    links: [
      { label: 'Open Jobs', path: JOBS_PATH },
      { label: 'Open CVs', path: CV_DOCUMENTS_PATH },
      { label: 'Open Import CV', path: IMPORT_CV_PATH },
    ],
  },
  {
    id: 'first-steps',
    nav: 'Your first profile',
    title: 'Your first profile in six steps',
    promise: 'About ten minutes, and the AI can do most of the typing.',
    captions: [],
    click: [],
    links: [],
  },
  {
    id: 'careful',
    nav: 'Careful',
    title: 'What is permanent, and what is not saved until you say so',
    promise: 'Three things to know before you start deleting or importing.',
    captions: [
      {
        title: 'Deleting a profile is permanent',
        text: 'Deleting a profile permanently deletes its target roles and everything they own (details, skills, experience), with no further confirmation.',
      },
      {
        title: 'An import is a proposal',
        text: 'Import CV proposes a profile, a target role, a summary, skills, experience and education. Nothing is saved until you have reviewed it.',
      },
      {
        title: 'The AI only suggests',
        text: 'When the AI suggests a better text, you approve or decline each suggestion; declining deletes it.',
      },
    ],
    click: [],
    links: [{ label: 'Open Settings → AI', path: SETTINGS_PATH }],
  },
]

// The parts of the completeness bar, in the order the backend lists them, each with its weight (they add up to 100) and what it needs.
export interface CompletenessPart {
  id: string
  label: string
  weight: number
  needs: string
}

export const COMPLETENESS_PARTS: CompletenessPart[] = [
  { id: 'name', label: 'Name', weight: 10, needs: 'A first or a last name' },
  { id: 'links', label: 'Platform links', weight: 25, needs: 'At least one link' },
  { id: 'image', label: 'Profile image', weight: 10, needs: 'A photo' },
  { id: 'persona', label: 'Target role', weight: 15, needs: 'At least one target role' },
  { id: 'summary', label: 'Headline and summary', weight: 15, needs: 'A target role with both' },
  { id: 'skills', label: 'Skills', weight: 15, needs: 'At least three' },
  { id: 'experience', label: 'Experience', weight: 10, needs: 'At least one entry' },
]

// The four words next to the percentage, from the lowest band up, with where each starts.
export const COMPLETENESS_BANDS: { word: string; from: number }[] = [
  { word: 'Just started', from: 0 },
  { word: 'Getting there', from: 40 },
  { word: 'Strong', from: 70 },
  { word: 'Complete', from: 100 },
]

// What a target role follows from the profile and what is its own: the rows of the table in the chapter "Target roles".
export interface RoleRow {
  id: string
  what: string
  how: 'follows' | 'chooses' | 'own' | 'profile'
  note: string
}

export const ROLE_ROWS: RoleRow[] = [
  { id: 'summary', what: 'Summary', how: 'follows', note: 'The profile’s, until the role has its own wording' },
  { id: 'goals', what: 'Goals', how: 'follows', note: 'The profile’s, unless the role has a goal of its own' },
  { id: 'skills', what: 'Skills', how: 'chooses', note: 'Picked from the profile’s library, or the role’s own' },
  { id: 'experience', what: 'Experience', how: 'chooses', note: 'Picked from the library, can be reworded, or the role’s own' },
  { id: 'headline', what: 'Headline', how: 'own', note: 'Always the role’s own' },
  { id: 'location', what: 'Location', how: 'own', note: 'Always the role’s own' },
  { id: 'rest', what: 'Education, languages, courses, hobbies', how: 'profile', note: 'The profile’s, shared by every role' },
]

// Example roles for the picture of chapter "Target roles".
export const EXAMPLE_ROLES = ['Backend Engineer', 'Team Lead', 'Data Engineer']

// The short part for a page with no profile (the teaser) and the card in the left column.
export const TEASER_TITLE = 'What are profiles for?'
export const TEASER_LEAD = 'A profile is one job seeker. Career builds your jobs, CVs and applications from it.'
export const TEASER_STEPS: { id: string; title: string; text: string }[] = [
  { id: 'profile', title: 'Create a profile', text: 'One for each job seeker.' },
  { id: 'role', title: 'Add target roles', text: 'One for each kind of job.' },
  { id: 'data', title: 'Fill in your data', text: 'Skills, experience, education.' },
]
export const INFO_BUTTON = 'See how profiles work'
export const HEADER_BUTTON = 'How it works'
export const CARD_TITLE = 'How profiles work'
export const CARD_BUTTON = 'See how it works'
export const CARD_TEXT = 'Profile, target roles, your data and the jobs, in one picture.'
export const MODAL_TITLE = 'How profiles work'

// The steps of "Your first profile". `track` ties a step to what the page can tell (see profileInfoRules.journeyState); null = it cannot.
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
    text: 'Import CV, matching and CVs use the AI platform and model chosen in Settings.',
    click: 'Settings → AI',
    track: null,
  },
  { id: 'profile', title: 'Create the profile', text: 'Or let the AI read a CV into a new one.', click: 'New profile', track: 'profile' },
  {
    id: 'links',
    title: 'Add a platform link and a photo',
    text: 'A link such as LinkedIn is the biggest part of completeness.',
    click: '+ Platform',
    track: 'links',
  },
  { id: 'role', title: 'Add a target role', text: 'One for each kind of job you want.', click: 'New target role', track: 'role' },
  {
    id: 'content',
    title: 'Write the summary, add skills and experience',
    text: 'Summary, Skills and Experience are on the profile’s card.',
    click: 'Summary, Skills, Experience',
    track: 'content',
  },
  { id: 'cv', title: 'Create a CV, look at the jobs', text: 'A CV is made from a target role.', click: 'CVs', track: null },
]
