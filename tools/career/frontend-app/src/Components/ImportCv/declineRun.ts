import { updateCVImportRunSaveSummary } from '../../api'
import { buildDeclinedSummary } from './runOutcome'

// Declines an import (plan/ai/tools/career/step-242-restart-cv-import-inside-the-import-page.md, step 2): records, through the route that already records the
// outcome of a review, that nothing was taken. Only for a run nothing was saved from (the review and failed views); the caller checks that.
export async function declineRun(runId: string): Promise<void> {
  await updateCVImportRunSaveSummary(runId, buildDeclinedSummary())
}
