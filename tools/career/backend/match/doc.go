// Package match holds the parts of matching a job that the persona side and the profile side share: the manual and AI Job Match, the
// paging of job ids, and the purge of the matches a deleted profile or persona leaves behind. The persona side is the sub-package persona
// (package personamatching) and the profile side the sub-package profile (package profilematch). The other sub-packages are the pure
// scoring engine (jobmatcher), the stored persona scores (personamatch), the debounced re-match worker and the catch-up of deletions
// (rematchworker, matchorphans), and the event consumers of the found, changed, created and deleted jobs, profiles and personas
// (consumer). The routes that call it are in jobs/handlers, persona/handlers and profile/handlers.
package match
