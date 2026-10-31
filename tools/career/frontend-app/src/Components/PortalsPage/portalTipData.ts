// The text of the tips shown beside the portals (step 105). Plain data, no React, so it can be
// script-checked: a tip is a short title and a short text, written for a 224px column, with no
// prices. Editorial text, not configuration: it changes with what the app does, so it lives here
// and not in the component. See plan/ai/tools/career/step-105-shared-carousel-and-portal-tips.md.

import { ACCENTS, type Accent } from '../JobsPage/jobsTheme'

export type PortalTipTone = 'amber' | 'rose' | 'green' | 'violet'

// A tip's colour: the dashboards' amber, rose, green and violet.
export function toneAccent(tone: PortalTipTone): Accent {
  switch (tone) {
    case 'amber':
      return ACCENTS[2]
    case 'rose':
      return ACCENTS[3]
    case 'green':
      return ACCENTS[1]
    case 'violet':
      return ACCENTS[4]
  }
}

export interface PortalTipData {
  id: string
  // The dot's tooltip and what a screen reader hears.
  label: string
  title: string
  text: string
  tone: PortalTipTone
  // A tip can point at Settings.
  settingsLink?: boolean
}

// The most words a tip's text may have: the column is narrow and a tip is read at a glance.
export const MAX_TIP_WORDS = 30

export function wordCount(text: string): number {
  const words = text.trim().split(/\s+/)
  return words.length === 1 && words[0] === '' ? 0 : words.length
}

// The order is the order they are shown in: the choice of AI first, then cheaper models, then what cannot be crawled.
export const PORTAL_TIP_DATA: PortalTipData[] = [
  {
    id: 'your-ai',
    label: 'Your AI',
    title: 'Your choice of AI',
    text: 'A portal or a link can use its own AI platform and model (the AI chip). With none set, the portal’s is used, then the general one in Settings.',
    tone: 'violet',
    settingsLink: true,
  },
  {
    id: 'cheaper-models',
    label: 'Cheaper models',
    title: 'Cheaper models can do it',
    text: 'Low-cost models from OpenRouter can often write crawl instructions too. Pick a specific cheap model: the random free router often fails at tool use.',
    tone: 'green',
  },
  {
    id: 'not-every-page',
    label: 'Not every page',
    title: 'Not every page can be crawled',
    text: 'Some sites block automated visits or load their jobs only after a click or scroll. After 3 failed tries a link shows “Probably can’t be crawled”.',
    tone: 'amber',
  },
  {
    id: 'paywalls',
    label: 'Paywalls',
    title: 'Paywalls and logins',
    text: 'Pages behind a paywall or a login cannot be crawled. Use the site’s public job list instead.',
    tone: 'rose',
  },
]
