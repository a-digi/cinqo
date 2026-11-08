// The Katana engine's AI recipe for the JOB DETAILS (plan 192, step 8): the tools its hidden conversation may use, its budget and the prompt.
// Rod's job-detail prompt and tool list (generateInstructions.ts) are not touched. Keep the list in step with the tools the prompt names, and the
// message well under core's 8,000-byte limit.
import type { ListingGenerate } from '../types'

export const KATANA_JOB_DETAIL_TOOLS = [
  'katana_get_instruction_state',
  'katana_get_job_page_candidates',
  'katana_test_job_detail_picks',
  'katana_save_job_detail_picks',
] as const

// A tidy run needs four calls (state, candidates, test, save); the rest is room for one fix.
export const KATANA_JOB_DETAIL_ITERATION_LIMIT = 8

export function buildKatanaJobDetailMessage(link: { id: string; title?: string | null; url: string }): string {
  return [
    'Please pick where the data of a job page is, for this existing portal link only (its jobs were found by the Katana link crawler and have only a title and an address; the job page is where the company, place, date and description come from). Do not create a portal or a link and do not ask which link is meant (it is given below). You only use the katana_ tools; nothing here needs HTML.',
    '',
    `Portal link ID: ${link.id}`,
    `Title: ${link.title ?? '(untitled)'}`,
    `URL: ${link.url}`,
    '',
    `Tool-call budget: at most ${KATANA_JOB_DETAIL_ITERATION_LIMIT} tool calls in total, every call counts. A tidy run needs four.`,
    '',
    'Steps:',
    '1. Call katana_get_instruction_state with this link id FIRST and follow its nextStep. Only job_details means: go on. For any other nextStep change nothing, reply with its reason in one line and end. Never redo a part that is saved.',
    '2. Call katana_get_job_page_candidates. It loads the page of one crawled job and returns the best 3 places for the title, company, location, date and description (selector, a short value, the reasons), the page notices, what inside the description box looks like furniture, and `suggested`: the picks the top candidates make. A notice that the page is a reduced page or not the job means the site served something else to this visit: STOP and say so, do not pick.',
    '3. Look at the values: the title must be the job title (not the board heading or the site name), the company only the company name, the location only the place, the date a clean date, the description the whole posting. If `suggested` is right, use it; otherwise change only the wrong field, copying selectors exactly from the candidates. Add descriptionExclude (selectors from descriptionExcludes) or descriptionCutAfter only when the description holds furniture such as a share bar or similar jobs. Leave a field out when the page does not state it; never invent a selector.',
    '4. Call katana_test_job_detail_picks (leave the picks out to test `suggested`). It runs the real extraction on up to 3 crawled jobs and returns values, flags and a verdict. A verdict starting with ok: go on. "blocking" or a wrong value: make exactly ONE fix and test again.',
    '5. Only after a test with verdict ok, call katana_save_job_detail_picks ONCE (same picks, or none for `suggested`). It refuses picks that have not passed their test.',
    '',
    'STOP and do not save when: a tool says the site blocked the page (then say that Rod can be used for this link), a tool says the login of the link expired or is not kept, there is no crawled job, no signed-in session is known, or Katana is not the selected engine; or the test is still blocking after your one fix. Then reply with one short line quoting what the tool said or what you saw.',
    '',
    'When done, reply with one short line.',
  ].join('\n')
}

export const katanaJobDetailGenerate: ListingGenerate = {
  tools: KATANA_JOB_DETAIL_TOOLS,
  toolIterationLimit: KATANA_JOB_DETAIL_ITERATION_LIMIT,
  buildMessage: buildKatanaJobDetailMessage,
}
