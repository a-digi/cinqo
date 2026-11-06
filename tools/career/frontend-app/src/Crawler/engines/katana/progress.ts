// What the Katana engine shows while the AI instructs a link (plan 192, the user's request: it must be clear when the job list is done and when
// the job details are being done). Pure: it only reads the facts a link already carries, so it is script-checked.
import type { PortalLink } from '../../../api'
import { hasText } from '../linkFacts'

export type StepId = 'rule' | 'jobs' | 'details'
// done: saved; working: the AI (or a crawl) is on it right now; waiting: the AI will get to it; todo: nothing happens yet.
export type StepState = 'done' | 'working' | 'waiting' | 'todo'

export interface ProgressStep {
  id: StepId
  label: string
  state: StepState
  // A few words for the state ("saved", "the AI is picking the data of a job page", ...).
  note: string
}

export interface KatanaProgress {
  steps: ProgressStep[]
  aiRunning: boolean
  // The sentence above the steps.
  headline: string
}

export type ProgressLink = Pick<
  PortalLink,
  | 'linkRules'
  | 'lastCrawledAt'
  | 'hasActiveCrawlRun'
  | 'jobDetailCrawlInstructions'
  | 'instructionsAiConversationId'
  | 'jobDetailInstructionsAiConversationId'
  | 'instructionStats'
>

export function katanaProgress(link: ProgressLink): KatanaProgress {
  const aiRunning =
    hasText(link.instructionsAiConversationId) ||
    hasText(link.jobDetailInstructionsAiConversationId) ||
    link.instructionStats.listing.running ||
    link.instructionStats.jobDetail.running
  const ruleDone = (link.linkRules ?? null) !== null
  const jobsDone = hasText(link.lastCrawledAt)
  const detailDone = hasText(link.jobDetailCrawlInstructions)
  const crawling = link.hasActiveCrawlRun

  const rule: ProgressStep = ruleDone
    ? { id: 'rule', label: 'Job list rule', state: 'done', note: 'saved' }
    : aiRunning
      ? { id: 'rule', label: 'Job list rule', state: 'working', note: 'the AI is writing it' }
      : { id: 'rule', label: 'Job list rule', state: 'todo', note: 'not written yet' }

  let jobs: ProgressStep
  if (!ruleDone) jobs = { id: 'jobs', label: 'Jobs crawled', state: 'todo', note: 'after the rule' }
  else if (crawling) jobs = { id: 'jobs', label: 'Jobs crawled', state: 'working', note: 'crawling now' }
  else if (jobsDone) jobs = { id: 'jobs', label: 'Jobs crawled', state: 'done', note: 'jobs saved' }
  else if (aiRunning) jobs = { id: 'jobs', label: 'Jobs crawled', state: 'working', note: 'the AI is crawling them' }
  else jobs = { id: 'jobs', label: 'Jobs crawled', state: 'todo', note: 'press Crawl now' }

  let details: ProgressStep
  if (detailDone) details = { id: 'details', label: 'Job details', state: 'done', note: 'saved' }
  else if (aiRunning && ruleDone && jobsDone && !crawling) {
    details = { id: 'details', label: 'Job details', state: 'working', note: 'the AI is picking the data of a job page' }
  } else if (aiRunning) details = { id: 'details', label: 'Job details', state: 'waiting', note: 'after the jobs' }
  else details = { id: 'details', label: 'Job details', state: 'todo', note: jobsDone ? 'not picked yet' : 'needs crawled jobs' }

  const steps = [rule, jobs, details]
  const current = steps.find((s) => s.state === 'working')
  const next = steps.find((s) => s.state !== 'done')
  let headline: string
  if (aiRunning && current) headline = `The AI is working on: ${current.label.toLowerCase()}`
  else if (aiRunning) headline = 'The AI is working on this link'
  else if (!next) headline = 'This link is instructed: job list, jobs and job details are done'
  else headline = `Next: ${next.label.toLowerCase()}`
  return { steps, aiRunning, headline }
}
