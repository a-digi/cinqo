// generateInstructions.ts (step 33) — mirrors crawl.ts's own shape
// exactly, for a different goal: writing this link's own stored
// crawl_instructions, never running a crawl or saving jobs. The
// conversation this message is sent to is created hidden
// (coreApi.ts's createConversation, hidden: true) — it never shows up
// in the normal conversation list
// (plan/ai/conversation/step-30-hidden-conversations.md) — so, unlike
// crawl.ts's own buildCrawlMessage, this reply is never read by any
// user; PortalsPage.tsx only ever surfaces whether the attempt failed.
// See plan/ai/tools/career/step-33-ai-generated-crawl-instructions.md.
export function buildGenerateInstructionsMessage(link: { id: string; title: string | null; url: string }): string {
  return [
    'Please create or update the stored crawl instructions for this existing portal link only — do not create a new portal or link, and do not ask which link is meant (it is already specified below). Do not run a crawl or save any jobs — this is only about the stored instructions themselves.',
    '',
    `Portal link ID: ${link.id}`,
    `Title: ${link.title ?? '(untitled)'}`,
    `URL: ${link.url}`,
    '',
    'Steps:',
    '1. Call get_portal_link_crawl_instructions for this link id to see whatever is currently stored (may be empty).',
    '2. Navigate to the URL (fetch_page_html) and inspect the page: does it list MULTIPLE similar items at once (a search-results/job-listing page), or does it describe a single item (a one-job detail page)?',
    "3. For a listing page, always include a top-level container (a CSS selector for one item's own repeating wrapping element) so results group correctly — the normal, expected shape for a listing page, not something to add only if something looks wrong.",
    '4. Work out the fields (and, for a listing page, mapping to title/url/company/location/description/postedAt where possible) and, if the page paginates, a pagination block (nextSelector + a reasonable maxPages).',
    '5. Call set_portal_link_crawl_instructions ONCE with the finished YAML for this exact portal link id.',
    '',
    'When done, reply with a short one-line confirmation — this reply is never shown to any user, so keep it brief.',
  ].join('\n')
}

export function generateInstructionsConversationTitle(link: { title: string | null; url: string }): string {
  return `Generate crawl instructions: ${link.title ?? link.url}`
}
