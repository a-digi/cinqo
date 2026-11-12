// Which profile an import goes into (plan 261): the pure rules for the "Import into" choice and its place in the address
//
//   /tools/career/import-cv                       a new profile is made from the CV
//   /tools/career/import-cv?profileId=<id>        the CV is imported into that profile
//
// Reading is forgiving (the first value counts, cleaned like every text in an address, an id that is no profile of the person is
// ignored); writing is canonical (the one parameter, only when a profile is chosen).
import { cleanText } from '../../Shared/AddressState/addressText'

// The profile id the address asks for, or '' for none.
export function parseImportTarget(search: string): string {
  return cleanText(new URLSearchParams(search).get('profileId'))
}

// "" for a new profile, or "?profileId=<id>" (a space written as %20, anything else encoded).
export function buildImportSearch(profileId: string): string {
  const id = cleanText(profileId)
  if (id === '') return ''
  return `?${new URLSearchParams({ profileId: id }).toString().replace(/\+/g, '%20')}`
}

// True when the address is already exactly what the page would write for the profile it means.
export function isCanonicalImportSearch(search: string): boolean {
  const withMark = search !== '' && !search.startsWith('?') ? `?${search}` : search
  return buildImportSearch(parseImportTarget(search)) === withMark
}

// The profile to import into: the wanted id when it is one of the profiles, otherwise '' (a new profile). While the profiles are not read yet
// (null) the wanted id stands, so nothing is thrown away before it can be checked; the server refuses an id that is no profile.
export function resolveImportTarget(wanted: string, profiles: { id: string }[] | null): string {
  if (wanted === '') return ''
  if (profiles === null) return wanted
  return profiles.some((p) => p.id === wanted) ? wanted : ''
}

// The target roles an existing-role picker offers: with a chosen profile only that profile's, otherwise all of them.
export function targetPersonaOptions<P extends { profileId: string }>(personas: P[], targetProfileId: string): P[] {
  return targetProfileId === '' ? personas : personas.filter((p) => p.profileId === targetProfileId)
}
