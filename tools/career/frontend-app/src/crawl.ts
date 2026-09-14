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
export function buildCrawlMessage(link: { id: string; title: string | null; url: string }): string {
  return [
    'Please run a manual crawl for this existing portal link only — do not create a new portal or link, and do not ask which link is meant (it is already specified below).',
    '',
    `Portal link ID: ${link.id}`,
    `Title: ${link.title || '(untitled)'}`,
    `URL: ${link.url}`,
    '',
    'Steps: call get_portal_link_crawl_instructions for this link id to load its stored crawl instructions, navigate to the URL, run crawl_paginated using those exact instructions, then call save_portal_job for every job found (with this portalLinkId). When done, reply with a short summary: how many jobs were found and saved, and how many were skipped as duplicates, or explain why none were found.',
  ].join('\n')
}

export function crawlConversationTitle(link: { title: string | null; url: string }): string {
  return `Crawl: ${link.title || link.url}`
}
