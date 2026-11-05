// Package match holds how a job is matched to a persona or a profile: the scorers (persona and profile), the manual and AI Job Match, the
// re-match of changed or new jobs, the fingerprints that tell when a persona or profile needs it again, and the purge of the matches a
// deleted profile or persona leaves behind. Its sub-packages are the pure scoring engine (jobmatcher), the stored persona scores
// (personamatch), the debounced re-match worker and the catch-up of deletions (rematchworker, matchorphans), and the event consumers of the
// found, changed, created and deleted jobs, profiles and personas. The routes that call it are in jobs/handlers, persona/handlers and
// profile/handlers.
package match
