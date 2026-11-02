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
// tells the agent (built from this constant), so the promise and the limit agree. A tidy run needs about 7 rounds;
// 14 leaves room for a model that wastes a few (the Indeed runs of steps 160 and 162 ended one round short of 10).
export const GENERATE_TOOL_ITERATION_LIMIT = 14

// The tools a "Generate instructions" conversation may use (plan/ai/conversation/step-45-ai-loop-token-economy.md, step 1): the server
// offers the model only these and refuses any other call (createConversation's allowedTools). Without it every request carried the
// definitions of all ~63 tools (about 19,000 tokens, 14,000 of them for tools a generation never calls) and the model was offered
// tools that save jobs or build CVs. Keep each list in step with the tools its prompt names: the check script fails when the prompt
// names a tool that is not in the list (the model would be told to call something that is refused) or the list has a tool the prompt
// never names. extract_page_data is named only to forbid it, so it is not offered.
export const GENERATE_LISTING_TOOLS = [
  'get_portal_link_crawl_instructions',
  'find_repeating_items',
  'fetch_page_html',
  'crawl_paginated',
  'set_portal_link_crawl_instructions',
  'ask_user_if_page_has_jobs',
] as const

export const GENERATE_JOB_DETAIL_TOOLS = [
  'get_portal_link_job_detail_crawl_instructions',
  'list_jobs',
  'fetch_page_html',
  'set_portal_link_job_detail_crawl_instructions',
] as const

export const GENERATE_RESTRICTION_NOT_APPLIED_MESSAGE =
  'This server could not limit the AI to the tools this run needs, so the run was not started. Update the Cinqo app and try again.'

// The whole message goes out as ONE chat message, which the core rejects above 8000 bytes (maxMessageContentLength in
// api/src/conversation/chat.go: "content exceeds maximum length"). Keep it well below that: the jobsConfirmed variant
// adds about 400 bytes and the link's own title and URL (often long) come on top. Measure before adding text.
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
          'IMPORTANT: the instructions stored for this link were used for a real crawl that found 0 jobs, and the user confirmed this page DOES list jobs. The stored instructions are wrong — do not keep them, find a different approach. The question "does this page have jobs?" is answered: do NOT call ask_user_if_page_has_jobs. If you still extract 0 items after your attempts, follow step 8 (save nothing, explain what you tried).',
          '',
        ]
      : []),
    'Please create or update the stored crawl instructions for this existing portal link only — do not create a new portal or link, and do not ask which link is meant (it is given below). Do not save any jobs yourself: only crawl_paginated test calls (step 5) and the final set_portal_link_crawl_instructions call are expected.',
    '',
    `Portal link ID: ${link.id}`,
    `Title: ${link.title ?? '(untitled)'}`,
    `URL: ${link.url}`,
    '',
    `Tool-call budget: at most ${GENERATE_TOOL_ITERATION_LIMIT} tool calls in total for this ENTIRE task; every call counts, whichever tool. Do not use extract_page_data or repeated fetch_page_html calls to probe selectors one by one. A long page's HTML is cut off long before its job list, so do not try to strip elements until the list appears: find_repeating_items (step 2) shows you the repeating job entries directly, and crawl_paginated (step 5; its result carries the verdict in "extractionCheck") is your only intended way to verify a selector. If you reach the budget without success, stop and follow step 8, even if you have not used all 4 attempts from step 6.`,
    '',
    'Tool arguments (anything else is rejected before it runs and still costs you a call): crawl_paginated takes ONLY "instructions", the YAML text: put pagination.maxPages (and the url, step 5) inside that YAML, never as a separate argument, and do not add portalLinkId. find_repeating_items takes "url" (optionally minItems / maxCandidates). fetch_page_html takes "url" and maxAttributeLength: 200; read further into a long page with "offset" (18,000 characters at a time, it tells you the next offset); removeSelectors, if you use it, is an ARRAY of strings (script, style, head and svg are already removed). Do not test the same draft twice: change something before you test again.',
    '',
    'Steps:',
    '1. Call get_portal_link_crawl_instructions for this link id to see whatever is currently stored (may be empty).',
    '2. Call find_repeating_items with this link\'s URL — it loads the page and returns the best candidate repeating groups, each with a selector already verified against the live page, an item count, the first links, a short sampleText and ONE trimmed sampleHtml, plus nextCandidates: the likely next-page controls, each with a verified selector. Pick the candidate whose sampleText looks like ONE job posting (a title with company or location), not a menu, a filter, a "related searches" list or a footer. Its selector is your container; its sampleHtml shows one item\'s markup, from which you choose the field selectors. If it returns no candidates, the page may be a single-job page or not list items: look at it with fetch_page_html (maxAttributeLength: 200; read further with offset, not by removing elements) and decide whether it lists MULTIPLE similar items or describes a single item.',
    "3. For a listing page, always include a top-level container (a CSS selector for one item's own repeating wrapping element) so results group correctly. Use the selector find_repeating_items gave you as it is: it was checked against the live page.",
    "4. Work out the fields (for a listing page, map them to title/url/company/location/description/postedAt where possible) and a pagination block: use the selector of the FIRST entry of find_repeating_items' nextCandidates as nextSelector — do not invent one — with a reasonable maxPages for real crawls. With no nextCandidates the list is a single page (or loads more on scroll, which cannot be paged): set maxPages: 1 and any placeholder nextSelector, since the format requires one.",
    '5. TEST before saving — mandatory. First make sure the page is loaded for crawl_paginated: find_repeating_items uses its own browser tab, so right after it the browser is blank (crawl_paginated would answer with an error and cost a call). Put the URL of this link as a top-level `url:` line of the draft YAML for the test call (crawl_paginated then opens the page itself); leave that line out of the version you save in step 7. Call crawl_paginated with your draft (you may set pagination.maxPages: 1 for this test only; keep the real maxPages in the saved version), The result carries its own verdict in "extractionCheck" (ok, itemsFound, itemsSkipped, skippedSamples, message, and a warning when there are only 1 or 2 items): it is the exact rule a real crawl applies. Do not count items yourself and do not retype the result anywhere; extractionCheck is the one source of truth.',
    '6. If extractionCheck.ok is not true and itemsFound is above 0, you have not finished — fix your container/field selectors/mapping using skippedSamples (it shows the raw fields of items that came back empty) and repeat step 5. Try up to 4 times in total (1 initial attempt + 3 fix-and-retry cycles), but stop sooner if you hit the tool-call budget above first.',
    '6a. ZERO ITEMS: you cannot tell whether the selectors are wrong or the page simply has no jobs. If extractionCheck reports itemsFound: 0, make exactly ONE fix attempt (the container first — try another candidate from find_repeating_items — then the field selectors) and test again. If itemsFound is still 0, STOP: call ask_user_if_page_has_jobs once, with a one-line note of what you tried (e.g. "2 attempts, container .job-result matched nothing"), make no further tool calls, do NOT call set_portal_link_crawl_instructions, and end your turn with one short line. The user\'s answer arrives as a new message in this conversation. Ask at most once in this whole task, and never save instructions that extract 0 items.',
    '6b. LOW COUNT: if extractionCheck has a "warning" (only 1 or 2 items), ok: true is NOT enough to save. Compare the count with what find_repeating_items reported for your candidate (or the entries you saw on the page): if the page shows more, your container matches only a wrapper or part of the page: fix it and test again (a fix attempt in step 6). Save only when the extracted count matches the page.',
    '6c. EMPTY ROWS: extractionCheck.ok: true with a message about completely empty items (blankItems above 0) means ads or placeholder rows; a real crawl skips them. Do not keep testing: go on to step 7 and save (narrow the container with :has(your title selector) only with rounds to spare).',
    "7. Only once extractionCheck.ok is true (and any step 6b warning is resolved), call set_portal_link_crawl_instructions ONCE with the finished YAML (your real maxPages, not the test's maxPages: 1, and no url line) for this exact portal link id. Once ok: true appears, make this call next; do not test again.",
    '8. If you still cannot reach ok: true after 4 attempts (with itemsFound above 0), or you reach the tool-call budget, do NOT call set_portal_link_crawl_instructions. Reply at once saying specifically what blocks it (quote the skippedSamples you saw) instead of saving something broken or probing on.',
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
    `You have a fresh budget of at most ${GENERATE_TOOL_ITERATION_LIMIT} tool calls. You may look at the page once more: call find_repeating_items again (a different candidate than before is likely the right one) and, only if you need to read the page itself, fetch_page_html with maxAttributeLength: 200 (read further with offset, not by removing elements). You must look for a different approach than before: a different container, different field selectors, or content that a plain fetch does not contain because it is rendered by JavaScript (say so if that is what you find). Then continue from step 5: test with crawl_paginated (read extractionCheck), and call set_portal_link_crawl_instructions once, only when ok is true.`,
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
