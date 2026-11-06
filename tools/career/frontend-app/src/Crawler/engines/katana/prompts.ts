// The Katana engine's AI recipe for the JOB LIST (plan 192, step 6): the tools its hidden conversation may use, its tool-call budget and the
// prompt. Rod's prompt builders and tool lists (generateInstructions.ts) are not touched. Keep the tool list in step with the tools the prompt
// names (a scratch check does this: every tool named is in the list and every tool in the list is named), and the message well under core's
// 8,000-byte limit for one chat message.
import type { ListingGenerate } from '../types'

export const KATANA_LISTING_TOOLS = [
  'katana_get_instruction_state',
  'katana_get_page_links',
  'katana_propose_link_rule',
  'katana_test_link_rule',
  'katana_save_link_rule',
] as const

// A tidy run needs five calls (state, links, propose, test, save); the rest is room for one fix.
export const KATANA_TOOL_ITERATION_LIMIT = 10

export function buildKatanaListingMessage(link: { id: string; title?: string | null; url: string }): string {
  return [
    'Please write the job list rule for this existing portal link only, with the Katana link crawler: do not create a portal or a link and do not ask which link is meant (it is given below). The rule is the partial address that all job links of the page share. You only use the katana_ tools; nothing here needs HTML or selectors.',
    '',
    `Portal link ID: ${link.id}`,
    `Title: ${link.title ?? '(untitled)'}`,
    `URL: ${link.url}`,
    '',
    `Tool-call budget: at most ${KATANA_TOOL_ITERATION_LIMIT} tool calls in total, every call counts. A tidy run needs five.`,
    '',
    'Steps:',
    '1. Call katana_get_instruction_state with this link id FIRST and follow its nextStep. Only job_list_rule means: go on. For any other nextStep (done, crawl_jobs, job_details, wait, stop_login) change nothing, reply with its reason in one line and end. Never redo a part that is saved.',
    '2. Call katana_get_page_links. It returns the links of the page grouped by the shape of their address: shape, count, 3 sample addresses with link texts, and nextPageCandidates. The job links are the group whose samples are job postings (a title with company or place), usually the largest group whose last address part varies (# or *). A menu, a footer, a filter or a category is not a job group.',
    '3. Call katana_propose_link_rule with jobUrls: 2 to 5 addresses copied exactly from the samples of THAT ONE group. Add notJobUrls (samples of a similar group that are not jobs) only when the proposal matches non-jobs. Add nextPageUrls (copied from nextPageCandidates) only when they lead to the next page of this same list. It returns the rule, matchesOnPage, sampleTitles, needsConfirm and notes. If the sampleTitles are not job titles, the pick was wrong: pick another group.',
    '4. Call katana_test_link_rule (leave rule out to test the proposal). It loads the real page and at most one more and returns the jobs found. A verdict starting with ok: go on. "no jobs found": make exactly ONE fix (another group, or notJobUrls), propose again and test again.',
    '5. Only after a test with verdict ok, call katana_save_link_rule ONCE (leave rule out). It refuses a rule that has not passed its test.',
    '',
    'STOP and do not save when: a tool says the site blocked the page, the link needs a login, no signed-in session is known, or Katana is not the selected engine; or the test still finds no jobs after your one fix. Then reply with one short line quoting what the tool said or what you saw.',
    '',
    'When done, reply with one short line.',
  ].join('\n')
}

export const katanaListingGenerate: ListingGenerate = {
  tools: KATANA_LISTING_TOOLS,
  toolIterationLimit: KATANA_TOOL_ITERATION_LIMIT,
  buildMessage: buildKatanaListingMessage,
}
