// Pure rules for the Browser & Engine page's health strip: what light each part shows.
// No React, so they can be script-checked.
import type { BrowserSettings, ChromeStatus, EngineStatusResponse } from './api'
import { ACCENTS, type Accent } from './Shared/theme'

export type Tone = 'ok' | 'bad' | 'unknown'

export interface Light {
  tone: Tone
  label: string
}

// Chrome's light: green when installed, red when not, grey while checking or when the
// status could not be read.
export function chromeLight(status: ChromeStatus | null, error: string, checking: boolean): Light {
  if (status) return status.installed ? { tone: 'ok', label: 'Installed' } : { tone: 'bad', label: 'Not installed' }
  if (error) return { tone: 'unknown', label: 'Could not check' }
  return { tone: 'unknown', label: checking ? 'Checking…' : 'Unknown' }
}

export interface EngineLight extends Light {
  name: 'Rod' | 'Playwright'
  // The engine the settings select (a backend that sends none means Rod, the default).
  selected: boolean
  detail: string
}

export function engineLights(status: EngineStatusResponse | null, settings: BrowserSettings | null): EngineLight[] {
  const playwrightSelected = settings?.fetchEngine === 'playwright'
  const light = (available: boolean | undefined): Light =>
    available === undefined
      ? { tone: 'unknown', label: 'Unknown' }
      : available
        ? { tone: 'ok', label: 'Available' }
        : { tone: 'bad', label: 'Not available' }
  return [
    { name: 'Rod', selected: !playwrightSelected, detail: status?.rod.detail ?? '', ...light(status?.rod.available) },
    { name: 'Playwright', selected: playwrightSelected, detail: status?.playwright.detail ?? '', ...light(status?.playwright.available) },
  ]
}

// The header pill: how the page's first question is answered, or nothing while unknown.
export function headerPill(light: Light): { text: string; accent: Accent } | undefined {
  if (light.tone === 'ok') return { text: 'Chrome ready', accent: ACCENTS[1] }
  if (light.tone === 'bad') return { text: 'Chrome missing', accent: ACCENTS[3] }
  return undefined
}
