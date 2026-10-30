// Pure rules for the engine checkboxes on Browser & Engine. The setting holds exactly one
// engine, so the two boxes work as a pair: one is always checked, and the way to change it is
// to check the other. No React, so they can be script-checked.
import type { FetchEngine } from './api'

export interface EngineChoice {
  // The engine the setting should have after the click.
  engine: FetchEngine
  // True when the click only tried to uncheck the selected engine: nothing changes, and the
  // page says one engine has to stay selected.
  refused: boolean
}

// What a click on one engine's checkbox does. Checking an engine selects it (the other box
// unchecks); unchecking the selected one is refused, so there is never none selected.
export function engineChoice(current: FetchEngine, clicked: FetchEngine, checked: boolean): EngineChoice {
  if (checked) return { engine: clicked, refused: false }
  return { engine: current, refused: clicked === current }
}

// Whether an engine's box can be checked: Playwright needs its driver, so while it is missing
// and not already selected it stays locked. Rod, and anything already selected, is never locked.
export function engineLocked(engine: FetchEngine, current: FetchEngine, playwrightAvailable: boolean | null): boolean {
  return engine === 'playwright' && current !== 'playwright' && playwrightAvailable === false
}
