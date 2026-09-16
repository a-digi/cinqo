// truncate (step 57) — display-only text shortening, e.g. for the
// Jobs page's own Location filter, whose values are raw, unbounded
// crawled text (see plan/ai/tools/career/step-55-jobs-page-location-
// dropdown-active-filters-live-search.md's own data-quality note).
// Never apply to a value actually sent to the backend — only to what's
// rendered.
export function truncate(text: string, maxLength: number): string {
  return text.length > maxLength ? text.slice(0, maxLength) + '…' : text
}
