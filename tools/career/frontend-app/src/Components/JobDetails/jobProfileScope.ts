// The profile a job page is opened for (step 153). The job board links a job with the board's profile in the address,
//
//   /tools/career/job-details?id=<jobId>&profileId=<profileId>
//
// and the page then shows only the comments written for that profile (the comments carry it as their metadata). Pages that open a
// job without a profile (the Jobs list, a portal, a company) have no profileId and show every comment. Reading is forgiving: the
// first profileId counts, and anything that is not UUID-shaped is no profile at all.
import { JOB_DETAILS_PATH } from '../../paths'

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

// The profile id of an address's query string, lower-cased (the form ids are stored in), or '' when there is none or it is not a UUID.
export function profileIdFromSearch(search: string): string {
  const raw = new URLSearchParams(search).get('profileId') ?? ''
  return UUID.test(raw) ? raw.toLowerCase() : ''
}

// The metadata that scopes the comments of a job page to a profile, or undefined for no scope.
export function commentScopeFor(profileId: string): Record<string, string> | undefined {
  return profileId === '' ? undefined : { profileId }
}

// The query string of a job page: the job, and the profile when there is a (valid) one.
export function jobDetailsSearch(jobId: string, profileId = ''): string {
  const profile = profileIdFromSearch(`?profileId=${encodeURIComponent(profileId)}`)
  return `?id=${encodeURIComponent(jobId)}${profile === '' ? '' : `&profileId=${encodeURIComponent(profile)}`}`
}

// The address of a job's page, opened for a profile (the job board) or for nobody in particular.
export function jobDetailsPathFor(jobId: string, profileId = ''): string {
  return `${JOB_DETAILS_PATH}${jobDetailsSearch(jobId, profileId)}`
}
