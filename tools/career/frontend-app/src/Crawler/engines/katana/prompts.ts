// The Katana engine's AI recipe for the whole link, started by Generate on the job list (plan 192, step 6, widened by the user's request): the
// tools its hidden conversation may use, its tool-call budget and the prompt. Rod's prompt builders and tool lists (generateInstructions.ts) are not touched. Keep the tool list in step with the tools the prompt
// names (a scratch check does this: every tool named is in the list and every tool in the list is named), and the message well under core's
// 8,000-byte limit for one chat message.
import type { ListingGenerate } from '../types'

// The Generate button on the job list instructs the WHOLE link in one conversation (plan 192, the user's request): the job list rule, then the
// AI crawls the jobs itself and waits, then the job details are picked on a crawled job, all with the katana_ tools.
export const KATANA_LISTING_TOOLS = [
  'katana_get_instruction_state',
  'katana_get_page_links',
  'katana_propose_link_rule',
  'katana_test_link_rule',
  'katana_save_link_rule',
  'katana_crawl_jobs',
  'katana_get_job_page_candidates',
  'katana_test_job_detail_picks',
  'katana_save_job_detail_picks',
] as const

// A tidy run needs about ten calls (state, links, propose, test, save, crawl, candidates, test, save); the rest is room for fixes.
export const KATANA_TOOL_ITERATION_LIMIT = 18

export function buildKatanaListingMessage(link: { id: string; title?: string | null; url: string }): string {
  return [
    'Please instruct this existing portal link completely, with the Katana link crawler, in this one conversation: first its job list rule, then its job details. Do not create a portal or a link and do not ask which link is meant (it is given below). You only use the katana_ tools; nothing here needs HTML.',
    '',
    `Portal link ID: ${link.id}`,
    `Title: ${link.title ?? '(untitled)'}`,
    `URL: ${link.url}`,
    '',
    `Tool-call budget: at most ${KATANA_TOOL_ITERATION_LIMIT} tool calls in total, every call counts. A tidy run needs about ten.`,
    '',
    'Start with katana_get_instruction_state for this link id and follow its nextStep; call it again whenever you are unsure where the work stands. Never redo a part that is saved.',
    '- nextStep job_list_rule: do part A.',
    '- nextStep crawl_jobs: do part B.',
    '- nextStep job_details: do part C.',
    '- nextStep done, wait or stop_login: change nothing, reply with its reason in one line and end.',
    '',
    'A. The job list rule.',
    'A1. katana_get_page_links returns the links of the page grouped by address shape: shape, count, 3 sample addresses with link texts, nextPageCandidates. The job links are the group whose samples are job postings (a title with company or place), usually the largest group whose last address part varies (# or *). A menu, a footer, a filter or a category is not a job group.',
    'A2. katana_propose_link_rule with jobUrls: 2 to 5 addresses copied exactly from the samples of THAT ONE group. Add notJobUrls only when the proposal matches non-jobs, nextPageUrls (from nextPageCandidates) only when they lead to the next page of this same list. If the sampleTitles are not job titles, pick another group. If it refuses, correct the picks once; if it refuses twice, STOP.',
    'A3. katana_test_link_rule (leave rule out). Read its notes: if links of the same address shape are not matched and those are job postings, the rule is too narrow: propose again ONCE with addresses that differ more and test again. "no jobs found": make ONE fix and test again.',
    'A4. After a test with verdict ok, katana_save_link_rule ONCE (leave rule out). It refuses a rule that has not passed its test. Then go on with B in this same conversation.',
    '',
    'B. The jobs. The job details can only be picked on a crawled job, so call katana_crawl_jobs: it crawls the jobs with the saved rule and waits. status completed with jobs above 0: go on with C. running: call it again to keep waiting (at most twice). failed or 0 jobs: STOP and say what it said.',
    '',
    'C. The job details.',
    'C1. katana_get_job_page_candidates loads the page of one crawled job and returns the best 3 places for the title, company, location, date and description (selector, a short value, the reasons), page notices, what inside the description box looks like furniture, and `suggested`: the picks the top candidates make. A notice that the page is a reduced page or not the job: STOP and say so, do not pick.',
    'C2. Look at the values: the title must be the job title (not the board heading), the company only the company name, the location only the place, the date a clean date, the description the whole posting. If `suggested` is right use it; otherwise change only the wrong field, copying selectors exactly from the candidates. Add descriptionExclude or descriptionCutAfter only for furniture such as a share bar or similar jobs. Leave a field out when the page does not state it; never invent a selector.',
    'C3. katana_test_job_detail_picks (leave the picks out to test `suggested`) runs the real extraction on up to 3 crawled jobs. Verdict ok: go on. "blocking" or a wrong value: make ONE fix and test again.',
    'C4. After a test with verdict ok, katana_save_job_detail_picks ONCE. It refuses picks that have not passed their test. When it answers saved you are finished: reply with one short line and call no more tools.',
    '',
    'STOP and do not save when: a tool says the site blocked the page, the link needs a login, no signed-in session is known, or Katana is not the selected engine; or a test is still failing after your one fix. Then reply with one short line quoting what the tool said or what you saw.',
  ].join('\n')
}

export const katanaListingGenerate: ListingGenerate = {
  tools: KATANA_LISTING_TOOLS,
  toolIterationLimit: KATANA_TOOL_ITERATION_LIMIT,
  buildMessage: buildKatanaListingMessage,
}
