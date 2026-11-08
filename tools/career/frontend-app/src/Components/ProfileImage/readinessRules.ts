// Pure rules for the Photo page's CV-readiness checklist: what a profile still needs
// before a CV built from it looks complete. No React, so they can be script-checked.

export interface Readiness {
  hasPhoto: boolean
  // How many personas the profile has, and the first one's name (what is checked).
  personaCount: number
  // The first persona's summary text, skill count and experience count; null while that
  // persona's details are not known (there is no persona, or they could not be read).
  checked: { summary: string; skills: number; experience: number } | null
}

export type ReadinessKey = 'photo' | 'persona' | 'summary' | 'content'

export interface ReadinessItem {
  key: ReadinessKey
  label: string
  done: boolean
  // What is missing, or what is there; shown under the label.
  note: string
  // The page that fixes it, or null when there is nothing to open (the photo is on this
  // page; the later items wait for a persona).
  fix: 'new-persona' | 'details' | 'skills' | 'experience' | null
}

export function readinessItems(r: Readiness): ReadinessItem[] {
  const hasPersona = r.personaCount > 0
  const summaryDone = r.checked !== null && r.checked.summary.trim() !== ''
  const hasSkills = r.checked !== null && r.checked.skills > 0
  const hasExperience = r.checked !== null && r.checked.experience > 0

  const contentMissing = [hasSkills ? '' : 'skills', hasExperience ? '' : 'experience'].filter(Boolean).join(' and ')

  return [
    {
      key: 'photo',
      label: 'A photo',
      done: r.hasPhoto,
      note: r.hasPhoto ? 'Set.' : 'Upload one on the left.',
      fix: null,
    },
    {
      key: 'persona',
      label: 'A persona',
      done: hasPersona,
      note: hasPersona ? 'The CV is built from a persona.' : 'No persona yet.',
      fix: hasPersona ? null : 'new-persona',
    },
    {
      key: 'summary',
      label: 'A summary',
      done: summaryDone,
      note: !hasPersona ? 'Needs a persona first.' : r.checked === null ? 'Could not check.' : summaryDone ? 'Written.' : 'Still empty.',
      fix: hasPersona && r.checked !== null && !summaryDone ? 'details' : null,
    },
    {
      key: 'content',
      label: 'Skills and experience',
      done: hasSkills && hasExperience,
      note: !hasPersona
        ? 'Needs a persona first.'
        : r.checked === null
          ? 'Could not check.'
          : hasSkills && hasExperience
            ? `${r.checked.skills} skills, ${r.checked.experience} roles.`
            : `No ${contentMissing} yet.`,
      fix: hasPersona && r.checked !== null && !(hasSkills && hasExperience) ? (hasSkills ? 'experience' : 'skills') : null,
    },
  ]
}

export function readyCount(items: ReadinessItem[]): number {
  return items.filter((i) => i.done).length
}
