// Pure rules for the page of one import run (plan 166): which view a run gets. No React, so they can be script-checked.
import { isDeclinedRun } from './runOutcome'
import type { RunStatus } from './runStatusRules'

// 'review': a finished analysis nothing was saved from yet (the live checklist); 'ready': one that was saved from (read-only result); 'declined': one the
// person declined (nothing was saved; read-only, plan 242).
export type RunPageView = 'loading' | 'missing' | 'generating' | 'failed' | 'review' | 'ready' | 'declined'

// run is undefined while it loads and null when there is no such run (deleted, or a wrong address).
export function runPageView(run: { status: RunStatus; saveSummary?: unknown } | null | undefined): RunPageView {
  if (run === undefined) return 'loading'
  if (run === null) return 'missing'
  if (isDeclinedRun(run)) return 'declined'
  if (run.status === 'ready') return run.saveSummary === undefined || run.saveSummary === null ? 'review' : 'ready'
  return run.status
}

// An analysis that is still running is looked at again every few seconds; nothing else changes by itself.
export const RUN_POLL_INTERVAL_MS = 4000

export function shouldPollRun(view: RunPageView): boolean {
  return view === 'generating'
}
