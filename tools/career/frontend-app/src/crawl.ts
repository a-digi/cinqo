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
export function buildCrawlMessage(link: { id: string; title: string | null; url: string }): string {
  return [
    'Please run a manual crawl for this existing portal link only — do not create a new portal or link, and do not ask which link is meant (it is already specified below).',
    '',
    `Portal link ID: ${link.id}`,
    `Title: ${link.title || '(untitled)'}`,
    `URL: ${link.url}`,
    '',
    'Steps: call get_portal_link_crawl_instructions for this link id to load its stored crawl instructions, navigate to the URL, run crawl_paginated using those exact instructions, then call save_portal_jobs ONCE with every job you found (with this portalLinkId) — do not call save_portal_job job-by-job, that wastes tool-calling turns on a page with many jobs. When done, reply with a short summary: how many jobs were found and saved, and how many were skipped as duplicates, or explain why none were found.',
  ].join('\n')
}

export function crawlConversationTitle(link: { title: string | null; url: string }): string {
  return `Crawl: ${link.title || link.url}`
}
