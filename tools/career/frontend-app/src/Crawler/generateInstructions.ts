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
//
// Step 62 — step 2 now instructs maxAttributeLength: 200, to keep this
// call's own token cost down on pages with long inline noise.
// Best-effort, not a guarantee — the Browser tool has no way to force
// what arguments the AI actually passes to its own tool calls, unlike
// svg removal (now a Browser-tool default applying regardless of this
// prompt). See
// plan/ai/tools/career/step-62-svg-removal-and-attribute-length-enforcement.md.
export function buildGenerateInstructionsMessage(link: { id: string; title: string | null; url: string }): string {
  return [
    'Please create or update the stored crawl instructions for this existing portal link only — do not create a new portal or link, and do not ask which link is meant (it is already specified below). Do not save any jobs yourself — only crawl_paginated calls made to TEST your draft (step 5 below) and the final set_portal_link_crawl_instructions call are expected.',
    '',
    `Portal link ID: ${link.id}`,
    `Title: ${link.title ?? '(untitled)'}`,
    `URL: ${link.url}`,
    '',
    'Steps:',
    '1. Call get_portal_link_crawl_instructions for this link id to see whatever is currently stored (may be empty).',
    '2. Navigate to the URL (fetch_page_html — always pass maxAttributeLength: 200) and inspect the page: does it list MULTIPLE similar items at once (a search-results/job-listing page), or does it describe a single item (a one-job detail page)?',
    "3. For a listing page, always include a top-level container (a CSS selector for one item's own repeating wrapping element) so results group correctly — the normal, expected shape for a listing page, not something to add only if something looks wrong.",
    '4. Work out the fields (and, for a listing page, mapping to title/url/company/location/description/postedAt where possible) and, if the page paginates, a pagination block (nextSelector + a reasonable maxPages for real future crawls).',
    '5. TEST before saving — this is mandatory, not optional: call crawl_paginated with your draft instructions (you may temporarily set pagination.maxPages: 1 for this test call only, to keep it fast — testing page 1 is enough to judge whether your container/field selectors actually work; keep the real maxPages value for the version you eventually save in step 7), then call check_crawl_extraction_result with that exact response\'s own "pages" array. Do not count skipped/extracted items yourself by reading the raw JSON — check_crawl_extraction_result\'s own ok/itemsSkipped/skippedSamples is the one source of truth for whether your draft actually works.',
    '6. If ok is not true, you have not finished — fix your container/field selectors/mapping using skippedSamples (it shows the raw fields of items that came back empty) and repeat step 5. Try up to 4 times in total (1 initial attempt + 3 fix-and-retry cycles).',
    '7. Only once check_crawl_extraction_result reports ok: true, call set_portal_link_crawl_instructions ONCE with the finished YAML (using your real, intended maxPages — not the maxPages: 1 you may have used only for testing) for this exact portal link id.',
    '8. If you still cannot reach ok: true after 4 attempts, do NOT call set_portal_link_crawl_instructions — these instructions are not ready. Reply explaining specifically what is blocking it (quote the skippedSamples you saw) instead of saving something broken.',
    '',
    'When done, reply with a short one-line confirmation (or, per step 8, a short explanation of why it could not be finished) — this reply is never shown to any user, so keep it brief.',
  ].join('\n')
}

export function generateInstructionsConversationTitle(link: { title: string | null; url: string }): string {
  return `Generate crawl instructions: ${link.title ?? link.url}`
}

// buildGenerateJobDetailInstructionsMessage mirrors
// buildGenerateInstructionsMessage above exactly in spirit — same
// hidden-conversation, "just write the stored instructions, don't run
// anything real" shape — but for the SEPARATE job_detail_crawl_
// instructions document (portals.go), which describes a single JOB's
// own detail page, not this link's own listing page. There is no job
// detail URL to inspect until at least one job has actually been
// saved against this link (a prior listing crawl), so step 1 finds
// one via list_jobs rather than being handed link.url directly — see
// plan/ai/tools/career/step-XX-job-detail-crawl-instructions.md.
export function buildGenerateJobDetailInstructionsMessage(link: { id: string; title: string | null; url: string }): string {
  return [
    'Please create or update the stored JOB DETAIL crawl instructions for this existing portal link only — do not create a new portal or link, and do not ask which link is meant (it is already specified below). Do not run a crawl or save/modify any jobs — this is only about the stored instructions themselves.',
    '',
    `Portal link ID: ${link.id}`,
    `Title: ${link.title ?? '(untitled)'}`,
    `Listing URL: ${link.url}`,
    '',
    'Steps:',
    '1. Call get_portal_link_job_detail_crawl_instructions for this link id to see whatever is currently stored (may be empty).',
    `2. Call list_jobs with portalLinkId: ${link.id} and limit: 1 to find one real, already-saved job for this link — this gives you a sample job detail page URL to inspect. If it returns no jobs, reply explaining that this link has no saved jobs yet (a listing crawl needs to run first) and stop here without calling set_portal_link_job_detail_crawl_instructions.`,
    "3. Navigate to that job's own sourceUrl (fetch_page_html — always pass maxAttributeLength: 200) and inspect it: this is a single JOB DETAIL page, not the listing page — find the element(s) that hold the actual job posting text (title, description, requirements, etc.), ignoring navigation, ads, unrelated sections, and footers.",
    '4. Work out the fields needed to extract ONLY that job-position-relevant text as plain text — no container, no pagination (a detail page is neither a repeating list nor paginated). The effective output must include a field labeled description covering the full job posting body text.',
    '5. Call set_portal_link_job_detail_crawl_instructions ONCE with the finished YAML for this exact portal link id.',
    '',
    'When done, reply with a short one-line confirmation — this reply is never shown to any user, so keep it brief.',
  ].join('\n')
}

export function generateJobDetailInstructionsConversationTitle(link: { title: string | null; url: string }): string {
  return `Generate job detail crawl instructions: ${link.title ?? link.url}`
}
