// Package profilematch is the profile side of matching a job: the profile scorer (NewProfileScorer), the fingerprints that tell when a
// profile needs a re-match, the re-match of one profile against every job (RematchProfile), the re-match of jobs whose text changed
// (RematchChangedJobs) and the auto-match of jobs that have no match yet (AutoMatchJobs). It uses the shared loaders and the lock of
// package match and is used by the event consumers, the re-match worker and the crawler's criteria; package match does not import it.
package profilematch
