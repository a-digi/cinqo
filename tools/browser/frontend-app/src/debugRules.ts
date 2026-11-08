// Pure rules for the Debug page's logging gauge. No React, so they can be script-checked.
import type { BrowserSettings } from './api'

export interface Layer {
  key: 'crawl' | 'html' | 'challenge'
  label: string
  detail: string
  // Actually being logged now (Debug active and, for the two extras, their own toggle on).
  on: boolean
  // Its own toggle is on but Debug is off, so nothing is logged yet.
  pending: boolean
}

// The three things that can be logged, bottom (base) to top (most detailed). The two extras
// only take effect while Debug is active, as the backend treats them.
export function loggingLayers(s: Pick<BrowserSettings, 'debugEnabled' | 'debugLogHtml' | 'debugLogChallenge'>): Layer[] {
  return [
    { key: 'crawl', label: 'Crawl logs', detail: 'What each crawl did', on: s.debugEnabled, pending: false },
    {
      key: 'html',
      label: 'Page HTML',
      detail: "Each page's raw HTML",
      on: s.debugEnabled && s.debugLogHtml,
      pending: !s.debugEnabled && s.debugLogHtml,
    },
    {
      key: 'challenge',
      label: 'Challenge dumps',
      detail: 'HTML while waiting on a check',
      on: s.debugEnabled && s.debugLogChallenge,
      pending: !s.debugEnabled && s.debugLogChallenge,
    },
  ]
}

// 0 when nothing is logged, otherwise how many of the three layers are on (1 to 3).
export function loggingLevel(s: Pick<BrowserSettings, 'debugEnabled' | 'debugLogHtml' | 'debugLogChallenge'>): 0 | 1 | 2 | 3 {
  return loggingLayers(s).filter((l) => l.on).length as 0 | 1 | 2 | 3
}

export function levelWord(level: number): string {
  return ['Off', 'Basic', 'Detailed', 'Everything'][level] ?? 'Off'
}
