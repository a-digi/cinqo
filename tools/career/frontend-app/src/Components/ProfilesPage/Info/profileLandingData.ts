// The layout and the extra words of the Profiles infographic in the landing look (plan 267). The chapters, their captions and the pictures stay those of
// plan 266 (profileInfoData.ts); this file only says how they are laid out as tiles, what the hero and the tab strip say, and the questions and cards at
// the end. The answers of the questions ARE the checked sentences of the "careful" chapter (taken from it, never retyped). No React.
import { GUIDE_PROFILE_PATH, importCvPathFor } from '../../../paths'
import { CHAPTERS, MODAL_TITLE, HERO_LEAD, type ChapterId } from './profileInfoData'

// The line icon of a tab; the picture component maps each key to one of the tool's icons.
export type TabIcon =
  | 'flow'
  | 'user'
  | 'target'
  | 'layers'
  | 'gauge'
  | 'briefcase'
  | 'steps'
  | 'help'
  | 'bolt'
  | 'link'
  | 'pencil'
  | 'education'
  | 'languages'
  | 'courses'
  | 'hobbies'
  | 'camera'
  | 'match'
  | 'pdf'
  | 'upload'
  | 'robot'
  | 'users'
  | 'eye'
  | 'spark'
  | 'lock'
  | 'plus'

// How the first tile of a chapter is laid out: `wide` puts the text at the left and the picture at the right (for a small picture); `stack` gives the
// picture the whole width under the text (for a wide one). `tallCaptions` is how many of the first captions are tall tiles with the diagonal shape; the
// rest are plain half tiles. A chapter with `faq` has no tiles: it ends the page with the questions and the cards.
export interface SectionLayout {
  id: ChapterId
  // The short label of the tab (it fits above a 7 rem tab).
  tab: string
  icon: TabIcon
  lead: 'wide' | 'stack'
  tallCaptions: number
  faq: boolean
}

export const SECTIONS: SectionLayout[] = [
  { id: 'overview', tab: 'Big picture', icon: 'flow', lead: 'stack', tallCaptions: 0, faq: false },
  { id: 'profile', tab: 'A profile', icon: 'user', lead: 'wide', tallCaptions: 0, faq: false },
  { id: 'roles', tab: 'Target roles', icon: 'target', lead: 'stack', tallCaptions: 0, faq: false },
  { id: 'data', tab: 'What goes in', icon: 'layers', lead: 'stack', tallCaptions: 0, faq: false },
  { id: 'completeness', tab: 'Completeness', icon: 'gauge', lead: 'stack', tallCaptions: 0, faq: false },
  { id: 'jobs', tab: 'Jobs and CVs', icon: 'briefcase', lead: 'stack', tallCaptions: 2, faq: false },
  { id: 'first-steps', tab: 'First steps', icon: 'steps', lead: 'stack', tallCaptions: 0, faq: false },
  { id: 'careful', tab: 'Good to know', icon: 'help', lead: 'stack', tallCaptions: 0, faq: true },
]

// The hero band: the big title is the modal's own, the lead is the infographic's, and two buttons scroll to a section.
export const LANDING_HERO = {
  title: MODAL_TITLE,
  lead: HERO_LEAD,
  ghost: 'PROFILE',
  start: { label: 'Start with step 1', section: 'first-steps' as ChapterId },
  picture: { label: 'See the big picture', section: 'overview' as ChapterId },
}

export const FAQ_TITLE = 'Good to know'

// The three questions, each answering with the sentence of the "careful" chapter at the same place.
export const FAQ_QUESTIONS: string[] = [
  'What happens when I delete a profile?',
  'Is an imported CV saved at once?',
  'Can the AI change my texts without asking?',
]

export interface LandingFaqItem {
  id: string
  question: string
  answer: string
}

export function faqItems(): LandingFaqItem[] {
  const captions = CHAPTERS.find((c) => c.id === 'careful')?.captions ?? []
  return FAQ_QUESTIONS.map((question, i) => ({ id: `q${String(i + 1)}`, question, answer: captions[i].text }))
}

// The two centred cards after the questions: a text and a button that opens a real page of the tool.
export interface LandingServiceCard {
  id: string
  title: string
  text: string
  button: string
  href: string
}

export const SERVICE_CARDS: LandingServiceCard[] = [
  {
    id: 'guide',
    title: 'Read the whole guide',
    text: 'The step-by-step guide to setting up a profile and turning it into a CV.',
    button: 'Open the guide',
    href: GUIDE_PROFILE_PATH,
  },
  {
    id: 'import',
    title: 'Start from a CV',
    text: 'Let the AI read a CV into a profile. You review everything before it is saved.',
    button: 'Open Import CV',
    href: importCvPathFor(),
  },
]

// The icon of each caption tile, by the caption's title (the icon says what the tile is about, so a tile is found at a glance). A caption that is not here
// takes the icon of its chapter. The drift guard test checks that every caption has one and that none is left over.
export const CAPTION_ICONS: Partial<Record<string, TabIcon>> = {
  'You describe yourself once': 'user',
  'Target roles say what you want': 'target',
  'The tool works from it': 'bolt',
  'One per person': 'user',
  'Everything hangs off it': 'layers',
  'Its card has a tile for every page': 'steps',
  'Links and photos': 'link',
  'One role for each kind of job': 'target',
  'It follows the profile': 'flow',
  'It chooses what it uses': 'eye',
  'Always its own': 'lock',
  'CVs come from a role': 'pdf',
  'The AI reads them': 'robot',
  'Summary and goals': 'pencil',
  Skills: 'spark',
  Experience: 'briefcase',
  Education: 'education',
  Languages: 'languages',
  Courses: 'courses',
  Hobbies: 'hobbies',
  Photos: 'camera',
  'The bar on each card': 'gauge',
  'The four words': 'help',
  'A link counts most': 'link',
  'Some parts need a target role': 'target',
  'A match score for each profile': 'match',
  'Auto-add rules': 'plus',
  'A job board and applications': 'briefcase',
  CVs: 'pdf',
  'Import a CV': 'upload',
  'Let the AI improve a text': 'robot',
}

export function captionIcon(chapter: string, title: string): TabIcon {
  return CAPTION_ICONS[title] ?? SECTIONS.find((s) => s.id === chapter)?.icon ?? 'help'
}
