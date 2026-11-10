// insertProposal.ts — wires the review checklist's "Insert" action to
// Career's existing create/update routes. No new backend endpoints;
// every call here already existed before Import CV. See
// plan/ai/tools/career/import-cv/step-07-insert-selected-items.md.
import {
  addEducation,
  addProfileExperience,
  addProfileSkill,
  createPersona,
  createProfile,
  fetchPersonaExperienceView,
  fetchPersonaSkillsView,
  fetchEducation,
  fetchPersonas,
  fetchProfileDetails,
  fetchProfileExperience,
  fetchProfileSkills,
  fetchProfiles,
  setPersonaExperienceUse,
  setPersonaSkillIds,
  updatePersonaDetails,
  updateProfileDetails,
} from '../../api'
import { hasChanges, importDetailsPlan } from '../GoalsPage/goalsRules'
import { educationInputOf, educationLabel, findEducation, ALREADY_THERE } from './educationInsertRules'
import { addedEntryId, findEntry, findSkill, mergeIds, mergeUses } from './libraryInsertRules'
import type { CVImportProposal } from './parseProposal'

export interface ItemResult {
  ok: boolean
  error?: string
  // Something to say besides "inserted" (an education entry the profile already had).
  note?: string
}

export interface InsertResult {
  profile?: ItemResult
  persona?: ItemResult
  personalDetails?: ItemResult
  skills: (ItemResult & { index: number; value: string })[]
  experience: (ItemResult & { index: number; label: string })[]
  // Missing in a summary stored before education existed.
  education: (ItemResult & { index: number; label: string })[]
}

export interface InsertOptions {
  // Required when checked.persona is false — the dropdown selection
  // from the plan's own gap-resolution design (an existing persona to
  // attach Personal Details/Skills/Experience to instead of a newly
  // created one).
  existingPersonaId: string | null
  // Set once a previous call already resolved these (whether by
  // creating or by falling back to an existing one) — when non-null,
  // this call skips profile/persona resolution entirely rather than
  // creating a second profile/persona on a retry. null on the first
  // call for a given review session.
  resolvedProfileId: string | null
  resolvedPersonaId: string | null
}

function message(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

// Plan 173, step 5: what a CV states about skills and experience belongs to the PROFILE, so each checked skill and experience goes into
// the persona's profile library (an item the library already has is reused, never added twice) and the persona then uses it; what the
// persona already used stays chosen, and what it has of its own is not touched.
//
// insertSelected resolves Profile -> Persona -> (Personal Details,
// Skills, Experience) in that order, since the latter three all need
// a real personaId to attach to. A failure on one item never aborts
// the rest — every checked item still gets attempted, so one bad row
// doesn't silently cost the user everything else they reviewed and
// approved; failures are collected in the returned InsertResult
// instead of thrown. Returns the resolved profileId/personaId so the
// caller can pass them back in on a retry (via InsertOptions) instead
// of re-resolving — the real safety property that makes calling this
// more than once for the same review safe (see step 7's own Open
// Question 2): once resolved, this function never creates a second
// profile/persona for the same session.
export async function insertSelected(
  proposal: CVImportProposal,
  checked: Record<string, boolean>,
  options: InsertOptions,
): Promise<InsertResult & { profileId: string | null; personaId: string | null }> {
  const result: InsertResult = { skills: [], experience: [], education: [] }

  let profileId: string | null = options.resolvedProfileId
  if (profileId === null) {
    if (checked.profile) {
      try {
        const created = await createProfile(proposal.profile.name, '')
        profileId = created.id
        result.profile = { ok: true }
      } catch (err) {
        result.profile = { ok: false, error: message(err) }
      }
    } else {
      try {
        const profiles = await fetchProfiles()
        if (profiles.length === 0) throw new Error('no existing profile found')
        profileId = profiles[0].id
      } catch (err) {
        result.profile = { ok: false, error: message(err) }
      }
    }
  }

  let personaId: string | null = options.resolvedPersonaId
  if (personaId === null) {
    if (checked.persona) {
      if (profileId === null) {
        result.persona = { ok: false, error: 'no profile to attach this target role to' }
      } else {
        try {
          const created = await createPersona(profileId, proposal.persona.name, proposal.persona.title)
          personaId = created.id
          result.persona = { ok: true }
        } catch (err) {
          result.persona = { ok: false, error: message(err) }
        }
      }
    } else {
      personaId = options.existingPersonaId
      if (personaId === null) {
        result.persona = { ok: false, error: 'no existing target role selected' }
      }
    }
  }

  if (checked.personalDetails) {
    if (personaId === null) {
      result.personalDetails = { ok: false, error: 'no target role to attach to' }
    } else {
      try {
        // Plan 174: a summary or goal the CV states fills the PROFILE's value where it has none; the persona keeps only what differs.
        const ownProfileId = (await fetchPersonas()).find((p) => p.id === personaId)?.profileId ?? profileId
        if (ownProfileId === null) throw new Error('no profile found for this target role')
        const plan = importDetailsPlan(proposal.personalDetails, await fetchProfileDetails(ownProfileId))
        if (hasChanges(plan.profilePatch)) await updateProfileDetails(ownProfileId, plan.profilePatch)
        await updatePersonaDetails(personaId, plan.personaArgs)
        result.personalDetails = { ok: true }
      } catch (err) {
        result.personalDetails = { ok: false, error: message(err) }
      }
    }
  }

  // The library the items go into is the one of the persona's own profile (an existing persona may belong to another profile than the
  // first one).
  let libraryProfileId: string | null = profileId
  if (personaId !== null) {
    try {
      libraryProfileId = (await fetchPersonas()).find((p) => p.id === personaId)?.profileId ?? profileId
    } catch {
      libraryProfileId = profileId
    }
  }

  const skillEntries = [...proposal.skills.entries()].filter(([i]) => checked[`skill-${String(i)}`])
  const addedSkillIds: string[] = []
  const skillRows: { index: number; value: string; id?: string; error?: string }[] = []
  for (const [i, skill] of skillEntries) {
    if (personaId === null || libraryProfileId === null) {
      skillRows.push({ index: i, value: skill.value, error: 'no target role to attach to' })
      continue
    }
    try {
      let library = await fetchProfileSkills(libraryProfileId)
      let existing = findSkill(library, skill.value)
      if (!existing) {
        library = await addProfileSkill(libraryProfileId, skill.value)
        existing = findSkill(library, skill.value)
      }
      if (!existing) throw new Error('the skill was not found in the library after adding it')
      addedSkillIds.push(existing.id)
      skillRows.push({ index: i, value: skill.value, id: existing.id })
    } catch (err) {
      skillRows.push({ index: i, value: skill.value, error: message(err) })
    }
  }
  let skillChoiceError: string | undefined
  if (personaId !== null && addedSkillIds.length > 0) {
    try {
      const view = await fetchPersonaSkillsView(personaId)
      await setPersonaSkillIds(personaId, mergeIds(view.used, addedSkillIds))
    } catch (err) {
      skillChoiceError = `added to the library, but not chosen for the target role: ${message(err)}`
    }
  }
  for (const row of skillRows) {
    const failure = row.error ?? (row.id !== undefined ? skillChoiceError : undefined)
    result.skills.push(
      failure === undefined
        ? { index: row.index, value: row.value, ok: true }
        : { index: row.index, value: row.value, ok: false, error: failure },
    )
  }

  const experienceEntries = [...proposal.experience.entries()].filter(([i]) => checked[`experience-${String(i)}`])
  const addedEntryIds: string[] = []
  const experienceRows: { index: number; label: string; id?: string; error?: string }[] = []
  for (const [i, exp] of experienceEntries) {
    const label = `${exp.title} at ${exp.company}`
    if (personaId === null || libraryProfileId === null) {
      experienceRows.push({ index: i, label, error: 'no target role to attach to' })
      continue
    }
    try {
      const before = await fetchProfileExperience(libraryProfileId)
      let id = findEntry(before, exp)?.id
      if (id === undefined) {
        const after = await addProfileExperience(libraryProfileId, {
          company: exp.company,
          title: exp.title,
          startDate: exp.startDate,
          endDate: exp.endDate,
          description: exp.description,
        })
        id = addedEntryId(before, after, exp)
      }
      if (id === undefined) throw new Error('the experience was not found in the library after adding it')
      addedEntryIds.push(id)
      experienceRows.push({ index: i, label, id })
    } catch (err) {
      experienceRows.push({ index: i, label, error: message(err) })
    }
  }
  let experienceChoiceError: string | undefined
  if (personaId !== null && addedEntryIds.length > 0) {
    try {
      const view = await fetchPersonaExperienceView(personaId)
      await setPersonaExperienceUse(personaId, mergeUses(view.used, addedEntryIds))
    } catch (err) {
      experienceChoiceError = `added to the library, but not chosen for the target role: ${message(err)}`
    }
  }
  for (const row of experienceRows) {
    const failure = row.error ?? (row.id !== undefined ? experienceChoiceError : undefined)
    result.experience.push(
      failure === undefined
        ? { index: row.index, label: row.label, ok: true }
        : { index: row.index, label: row.label, ok: false, error: failure },
    )
  }

  // Plan 175: education belongs to the PROFILE (the persona's own, as for the library items); an entry the profile already has is not added
  // again (the row says so), a refused entry (the server checks it again) is a row error and does not stop the others.
  for (const [i, e] of proposal.education.entries()) {
    if (!checked[`education-${String(i)}`]) continue
    const label = educationLabel(e)
    if (libraryProfileId === null) {
      result.education.push({ index: i, label, ok: false, error: 'no profile to attach to' })
      continue
    }
    try {
      if (findEducation(await fetchEducation(libraryProfileId), e)) {
        result.education.push({ index: i, label, ok: true, note: ALREADY_THERE })
        continue
      }
      await addEducation(libraryProfileId, educationInputOf(e))
      result.education.push({ index: i, label, ok: true })
    } catch (err) {
      result.education.push({ index: i, label, ok: false, error: message(err) })
    }
  }

  return { ...result, profileId, personaId }
}
