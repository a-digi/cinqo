// Pure rules for the Import from CV page's pipeline: which of the four stages is
// done, current, failed or still to come, from the page's own state. No React, so they
// can be script-checked.
export type PageStateName = 'upload' | 'analyzing' | 'review' | 'error'
export type StageStatus = 'done' | 'current' | 'failed' | 'todo'

export interface PipelineInput {
  state: PageStateName
  // The upload's own inline error (shown while the page stays in 'upload').
  uploadError: boolean
  uploading: boolean
  inserting: boolean
  // Items saved so far, and items whose save failed (counts over all attempts).
  insertedCount: number
  insertErrorCount: number
}

export interface Stage {
  key: 'upload' | 'analyze' | 'review' | 'save'
  label: string
  hint: string
  status: StageStatus
  // True while the stage is actively working (drawn animated).
  busy: boolean
}

export function pipelineStages(i: PipelineInput): Stage[] {
  const saving = i.inserting
  const savedSomething = i.insertedCount > 0
  const saveTouched = saving || savedSomething || i.insertErrorCount > 0

  const upload: StageStatus = i.state !== 'upload' ? 'done' : i.uploadError ? 'failed' : 'current'
  const analyze: StageStatus = i.state === 'upload' ? 'todo' : i.state === 'analyzing' ? 'current' : i.state === 'error' ? 'failed' : 'done'
  const review: StageStatus = i.state !== 'review' ? 'todo' : saveTouched ? 'done' : 'current'
  const save: StageStatus = i.state !== 'review' || !saveTouched ? 'todo' : saving ? 'current' : i.insertErrorCount > 0 ? 'failed' : 'done'

  return [
    { key: 'upload', label: 'Upload', hint: 'Choose a PDF', status: upload, busy: i.uploading },
    { key: 'analyze', label: 'Analyze', hint: 'The AI reads it', status: analyze, busy: analyze === 'current' },
    { key: 'review', label: 'Review', hint: 'Pick what to keep', status: review, busy: false },
    { key: 'save', label: 'Save', hint: 'Add it to your profile', status: save, busy: saving },
  ]
}
