// The entity types Career declares in manifest.json's "reviews" (core caches them at install): keep them in step (and the backend's
// reviews/purge.go). A type is spelled once, here.
export const REVIEW_TYPE_COMPANY = 'company'

export type ReviewType = typeof REVIEW_TYPE_COMPANY
