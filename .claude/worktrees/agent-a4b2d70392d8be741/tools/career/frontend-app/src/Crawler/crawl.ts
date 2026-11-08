// crawl.ts — the pure, no-React/no-fetch pieces of the manual crawl
// trigger (step 24): the instruction message sent to the AI. Kept
// separate from PortalsPage.tsx/coreApi.ts so this exact text is
// trivially reviewable and testable in isolation.
//
// Deliberately tells the AI to re-fetch the crawl instructions itself
// via get_portal_link_crawl_instructions rather than inlining the
// stored YAML into this message — the stored value stays the one
// source of truth the AI reads live, not a copy the frontend could
// let go stale. See plan/ai/tools/career/step-24-manual-crawl-trigger.md.
//
// Step 29 changed the last step from "save_portal_job for every job
// found" (one tool call per job) to save_portal_jobs (one call, the
// whole batch) — a page listing many jobs used to need one tool-
// calling round-trip per job, routinely exceeding the conversation's
// own maxToolIterations ceiling
// (conversation: model kept calling tools without a final reply). See
// plan/ai/tools/career/step-29-batch-save-portal-jobs.md.
//
// Step 30 taught the AI about container grouping only as a reactive
// fix ("if results look misaligned, add a container"). Revised here to
// be proactive instead — check for a listing page as a normal, upfront
// step, matching set_portal_link_crawl_instructions' own description
// (portals.go), not just something to reach for after noticing a
// problem. See plan/ai/tools/career/step-30-grouped-crawl-ingestion.md
// and step-31-proactive-container-guidance.md.
//
// Step 62 — step 2 now names fetch_page_html explicitly (it only ever
// said "Navigate to the URL" before) and instructs
// maxAttributeLength: 200, to keep this call's own token cost down on
// pages with long inline noise. Best-effort, not a guarantee — the
// Browser tool has no way to force what arguments the AI actually
// passes to its own tool calls, unlike svg removal (now a Browser-tool
// default applying regardless of this prompt). See
// plan/ai/tools/career/step-62-svg-removal-and-attribute-length-enforcement.md.
export function buildCrawlMessage(link: { id: string; title: string | null; url: string }): string {
  return [
    'Please run a manual crawl for this existing portal link only — do not create a new portal or link, and do not ask which link is meant (it is already specified below).',
    '',
    `Portal link ID: ${link.id}`,
    `Title: ${link.title ?? '(untitled)'}`,
    `URL: ${link.url}`,
    '',
    'Steps:',
    '1. Call get_portal_link_crawl_instructions for this link id to load its stored crawl instructions.',
    '2. Navigate to the URL (fetch_page_html — always pass maxAttributeLength: 200), then look at the page: does it list MULTIPLE similar items at once (a search-results/job-listing page — the common case), or does it describe a single item (a one-job detail page)?',
    "3. If it's a listing page and the loaded instructions do NOT already have a top-level container, call set_portal_link_crawl_instructions first to add one (a CSS selector for one item's own repeating wrapping element) before crawling — this is the normal, expected step for a listing page, not something to do only after noticing a problem. A single-item detail page needs no container.",
    '4. Run crawl_paginated using the (possibly just-updated) instructions.',
    '5. Call save_portal_jobs ONCE with every job you found (with this portalLinkId) — do not call save_portal_job job-by-job, that wastes tool-calling turns on a page with many jobs.',
    '',
    "If you skipped step 3 and crawl_paginated's own results come back as separate arrays that don't clearly correspond field-for-field to the same job (e.g. a title that doesn't obviously match the url next to it), add a container now via set_portal_link_crawl_instructions and re-run crawl_paginated before saving anything.",
    '',
    'When done, reply with a short summary: how many jobs were found and saved, and how many were skipped as duplicates, or explain why none were found.',
  ].join('\n')
}

export function crawlConversationTitle(link: { title: string | null; url: string }): string {
  return `Crawl: ${link.title ?? link.url}`
}
