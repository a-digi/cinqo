// Package personamatching is the persona side of matching a job: the persona scorer (NewPersonaScorer), the fingerprint that tells when a
// persona changed, and the recompute of one persona against the jobs (RecomputePersona, StalePersonas). It stores nothing itself; the
// scores live in package personamatch. It uses the shared loaders of package match and is used by the event consumers; package match does
// not import it.
package personamatching
