// The entity types Career declares in manifest.json's "comments" (core caches them at install):
// keep the two in step. A type is spelled once, here.
export const COMMENT_TYPE_JOB = 'job'
export const COMMENT_TYPE_PERSONA = 'persona'
export const COMMENT_TYPE_CV = 'cv'

export type CommentType = typeof COMMENT_TYPE_JOB | typeof COMMENT_TYPE_PERSONA | typeof COMMENT_TYPE_CV
