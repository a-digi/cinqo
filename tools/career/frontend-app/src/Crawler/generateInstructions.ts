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
// GENERATE_TOOL_ITERATION_LIMIT (step 93) is the tool-call budget enforced on
// every "Generate instructions" conversation. It is the same number the prompt
// tells the agent ("at most 10 tool calls"), so the promise and the limit agree.
export const GENERATE_TOOL_ITERATION_LIMIT = 10

export function buildGenerateInstructionsMessage(
  link: { id: string; title?: string | null; url: string },
  // jobsConfirmed (step 91): the stored instructions were already used for a
  // real crawl that found 0 jobs and the user confirmed the page DOES list
  // jobs — so they are wrong, and the agent must not ask the question again.
  options: { jobsConfirmed?: boolean } = {},
): string {
  return [
    ...(options.jobsConfirmed
      ? [
          'IMPORTANT: the instructions currently stored for this link were used for a real crawl that found 0 jobs, and the user has confirmed that this page DOES list jobs. The stored instructions are therefore wrong — do not keep them, find a different approach. The question "does this page have jobs?" is already answered: do NOT call ask_user_if_page_has_jobs. If you still extract 0 items after your attempts, follow step 8 (save nothing and explain what you tried).',
          '',
        ]
      : []),
    'Please create or update the stored crawl instructions for this existing portal link only — do not create a new portal or link, and do not ask which link is meant (it is already specified below). Do not save any jobs yourself — only crawl_paginated calls made to TEST your draft (step 5 below) and the final set_portal_link_crawl_instructions call are expected.',
    '',
    `Portal link ID: ${link.id}`,
    `Title: ${link.title ?? '(untitled)'}`,
    `URL: ${link.url}`,
    '',
    "Tool-call budget: you have at most 10 tool calls total for this ENTIRE task — every call counts toward it (get_portal_link_crawl_instructions, fetch_page_html, crawl_paginated, check_crawl_extraction_result, and anything else), not only the crawl_paginated/check_crawl_extraction_result pairs in step 5. Do not use extract_page_data, or repeated fetch_page_html calls, to iteratively probe or guess individual selectors one attribute at a time. A long page's HTML is cut off long before its job list, so reading raw HTML shows you mostly the header and filters: do not try to strip elements until the list appears — find_repeating_items (step 2) shows you the repeating job entries directly. crawl_paginated + check_crawl_extraction_result (step 5) is your only intended way to verify a selector actually works against the real page. If you reach this budget without success, stop immediately and follow step 8, even if you have not used all 4 attempts from step 6 yet.",
    '',
    'Tool arguments (a call with anything else is rejected before it runs and still costs you a call): crawl_paginated takes ONLY "instructions" — the YAML text; put pagination.maxPages inside that YAML, never as a separate argument, and do not add portalLinkId. check_crawl_extraction_result takes ONLY "pages" — the exact "pages" array returned by the crawl_paginated call you just made, passed through unchanged as an array (do not retype it or wrap it in quotes). find_repeating_items takes the "url" (and optionally minItems / maxCandidates). fetch_page_html takes "url" and maxAttributeLength: 200; to read further into a long page pass "offset" (it returns 18,000 characters from there, and tells you the next offset); removeSelectors, if you ever use it, is an ARRAY of strings — and script, style, head and svg are already removed for you. Do not test the same draft twice: if a test did not work, change something before you test again.',
    '',
    'Steps:',
    '1. Call get_portal_link_crawl_instructions for this link id to see whatever is currently stored (may be empty).',
    '2. Call find_repeating_items with this link\'s URL — it loads the page and returns the best candidate repeating groups, each with a selector already verified against the live page, an item count, the first links, a short sampleText and ONE trimmed sampleHtml, plus nextCandidates: the likely next-page controls, each with a verified selector. Pick the candidate whose sampleText looks like ONE job posting (a title with company or location), not a menu, a filter, a "related searches" list or a footer. Its selector is your container; its sampleHtml shows the markup of one item, from which you choose the field selectors inside it. If it returns no candidates, the page may be a single-job page or may not list items: then look at it with fetch_page_html (always pass maxAttributeLength: 200; a long page is shown only in part, so read further with offset instead of removing elements to guess) and decide whether it lists MULTIPLE similar items (a search-results/job-listing page) or describes a single item (a one-job detail page).',
    "3. For a listing page, always include a top-level container (a CSS selector for one item's own repeating wrapping element) so results group correctly — the normal, expected shape for a listing page, not something to add only if something looks wrong. Use the selector find_repeating_items gave you as it is: it was checked against the live page.",
    "4. Work out the fields (and, for a listing page, mapping to title/url/company/location/description/postedAt where possible) and, if the page paginates, a pagination block: use the selector of the FIRST entry of find_repeating_items' nextCandidates as nextSelector — do not invent one — with a reasonable maxPages for real future crawls. If it returned no nextCandidates, the list is a single page (or loads more on scroll, which cannot be paged): set maxPages: 1 and any placeholder nextSelector, since the format requires one.",
    '5. TEST before saving — this is mandatory, not optional: call crawl_paginated with your draft instructions (you may temporarily set pagination.maxPages: 1 for this test call only, to keep it fast — testing page 1 is enough to judge whether your container/field selectors actually work; keep the real maxPages value for the version you eventually save in step 7), then call check_crawl_extraction_result with that exact response\'s own "pages" array. Do not count skipped/extracted items yourself by reading the raw JSON — check_crawl_extraction_result\'s own ok/itemsSkipped/skippedSamples is the one source of truth for whether your draft actually works.',
    '6. If ok is not true and itemsFound is above 0, you have not finished — fix your container/field selectors/mapping using skippedSamples (it shows the raw fields of items that came back empty) and repeat step 5. Try up to 4 times in total (1 initial attempt + 3 fix-and-retry cycles), but stop sooner if you hit the tool-call budget above first.',
    '6a. ZERO ITEMS means something is wrong, and you cannot tell by yourself whether the selectors are wrong or the page simply has no jobs. If check_crawl_extraction_result reports itemsFound: 0, make exactly ONE fix attempt (the container selector first — try another candidate from find_repeating_items — then the field selectors) and test again. If itemsFound is still 0, STOP fixing: call ask_user_if_page_has_jobs once, with a one-line note of what you tried (e.g. "2 attempts, container .job-result matched nothing"), make no further tool calls, do NOT call set_portal_link_crawl_instructions, and end your turn with one short line. The user will answer yes or no; the answer arrives as a new message in this conversation. Ask at most once in this whole task, and never save instructions that extract 0 items.',
    '6b. LOW COUNT: if check_crawl_extraction_result returns a "warning" (only 1 or 2 items extracted), ok: true is NOT enough to save. Compare the count with the item count find_repeating_items reported for the candidate you chose (or the number of entries you saw on the page): if the page shows more than the tool extracted, your container selector matches only a wrapper or part of the page, so fix it and test again (this counts as a fix attempt in step 6). Save only when the extracted count matches what the page shows.',
    '7. Only once check_crawl_extraction_result reports ok: true (and any warning from step 6b is resolved), call set_portal_link_crawl_instructions ONCE with the finished YAML (using your real, intended maxPages — not the maxPages: 1 you may have used only for testing) for this exact portal link id.',
    '8. If you still cannot reach ok: true after 4 attempts (with itemsFound above 0), or you have reached the tool-call budget, do NOT call set_portal_link_crawl_instructions — these instructions are not ready. Reply immediately explaining specifically what is blocking it (quote the skippedSamples you saw) instead of saving something broken or continuing to probe.',
    '',
    'When done, reply with a short one-line confirmation (or, per step 6a, one line saying you asked the user; or, per step 8, a short explanation of why it could not be finished) — keep it brief.',
  ].join('\n')
}

// buildJobsConfirmedMessage (step 91) is sent into the SAME conversation
// after the user answered "yes, this page has jobs" to the question the
// agent asked in step 6a. It tells the agent its approach was wrong, gives it
// a fresh budget, and forbids asking again, so a second 0-item result ends
// as a reported failure (the existing error InfoBox and step 86's attempt
// count) instead of a second question.
export function buildJobsConfirmedMessage(link: { id: string; title?: string | null; url: string }): string {
  return [
    'The user confirmed that this page DOES list jobs, so your previous selectors and approach are wrong — 0 items is not a valid result for this page.',
    '',
    `Portal link ID: ${link.id}`,
    `Title: ${link.title ?? '(untitled)'}`,
    `URL: ${link.url}`,
    '',
    'You have a fresh budget of at most 10 tool calls. You may look at the page once more: call find_repeating_items again (a different candidate than before is likely the right one) and, only if you need to read the page itself, fetch_page_html with maxAttributeLength: 200 (read further with offset, not by removing elements). You must look for a different approach than before: a different container, different field selectors, or content that a plain fetch does not contain because it is rendered by JavaScript (say so if that is what you find). Then continue from step 5: test with crawl_paginated + check_crawl_extraction_result, and call set_portal_link_crawl_instructions once, only when ok is true.',
    'Do NOT call ask_user_if_page_has_jobs again. If you still extract 0 items, do not save anything: reply with a short explanation of what you tried and what the page looks like (this is shown to the user as the reason it failed).',
  ].join('\n')
}

export function generateInstructionsConversationTitle(link: { title?: string | null; url: string }): string {
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
export function buildGenerateJobDetailInstructionsMessage(link: { id: string; title?: string | null; url: string }): string {
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

export function generateJobDetailInstructionsConversationTitle(link: { title?: string | null; url: string }): string {
  return `Generate job detail crawl instructions: ${link.title ?? link.url}`
}
