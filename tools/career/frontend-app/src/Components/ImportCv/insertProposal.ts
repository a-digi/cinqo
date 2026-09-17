// insertProposal.ts — wires the review checklist's "Insert" action to
// Career's existing create/update routes. No new backend endpoints;
// every call here already existed before Import CV. See
// plan/ai/tools/career/import-cv/step-07-insert-selected-items.md.
import { createProfile, createPersona, updatePersonaDetails, addSkill, addExperience, fetchProfiles } from '../../api'
import type { CVImportProposal } from './parseProposal'

export interface ItemResult {
  ok: boolean
  error?: string
}

export interface InsertResult {
  profile?: ItemResult
  persona?: ItemResult
  personalDetails?: ItemResult
  skills: (ItemResult & { index: number; value: string })[]
  experience: (ItemResult & { index: number; label: string })[]
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
  const result: InsertResult = { skills: [], experience: [] }

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
        result.persona = { ok: false, error: 'no profile to attach this persona to' }
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
        result.persona = { ok: false, error: 'no existing persona selected' }
      }
    }
  }

  if (checked.personalDetails) {
    if (personaId === null) {
      result.personalDetails = { ok: false, error: 'no persona to attach to' }
    } else {
      try {
        await updatePersonaDetails(personaId, {
          headline: proposal.personalDetails.headline,
          summary: proposal.personalDetails.summary,
          location: proposal.personalDetails.location,
          desiredTitles: proposal.personalDetails.desiredTitles,
          desiredLocations: proposal.personalDetails.desiredLocations,
          minSalary: proposal.personalDetails.minSalary,
        })
        result.personalDetails = { ok: true }
      } catch (err) {
        result.personalDetails = { ok: false, error: message(err) }
      }
    }
  }

  for (const [i, skill] of proposal.skills.entries()) {
    if (!checked[`skill-${i}`]) continue
    if (personaId === null) {
      result.skills.push({ index: i, value: skill.value, ok: false, error: 'no persona to attach to' })
      continue
    }
    try {
      await addSkill(personaId, skill.value)
      result.skills.push({ index: i, value: skill.value, ok: true })
    } catch (err) {
      result.skills.push({ index: i, value: skill.value, ok: false, error: message(err) })
    }
  }

  for (const [i, exp] of proposal.experience.entries()) {
    if (!checked[`experience-${i}`]) continue
    const label = `${exp.title} at ${exp.company}`
    if (personaId === null) {
      result.experience.push({ index: i, label, ok: false, error: 'no persona to attach to' })
      continue
    }
    try {
      await addExperience(personaId, {
        company: exp.company,
        title: exp.title,
        startDate: exp.startDate,
        endDate: exp.endDate,
        description: exp.description,
      })
      result.experience.push({ index: i, label, ok: true })
    } catch (err) {
      result.experience.push({ index: i, label, ok: false, error: message(err) })
    }
  }

  return { ...result, profileId, personaId }
}
