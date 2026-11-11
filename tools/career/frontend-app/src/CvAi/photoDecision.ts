// photoDecision.ts (plan 256) — does an AI-made CV carry the person's photo? Yes when the profile has one, unless the note the person wrote asks for a CV without it. The note is
// read by plain rules, not by the model: a small model is not reliable at a negation, and the person must be able to count on "without photo". English, German and Albanian.

// "without photo", "no picture", "ohne Foto", "kein Bild", "pa foto", "not a photo", "photo-free": a negation (up to two other words in between) next to a word for a photo.
const PHOTO_WORD = String.raw`(?:photos?|pictures?|pics?|images?|headshots?|portraits?|fotos?|fotografi\w*|bilder?|bild|passbild\w*|lichtbild\w*|bewerbungsfoto\w*|imazh\w*)`
const NEGATION = String.raw`(?:without|w/o|no|not|don'?t\s+want|do\s+not\s+want|dont\s+want|skip|omit|exclude|leave\s+out|ohne|kein\w*|nicht|pa|nuk\s+dua|pa\s+u)`
const NO_PHOTO = new RegExp(
  String.raw`(?<![\p{L}\p{N}])${NEGATION}(?:\s+[\p{L}'-]+){0,2}?\s+${PHOTO_WORD}(?![\p{L}\p{N}])|(?<![\p{L}\p{N}])${PHOTO_WORD}[- ]?(?:free|less|frei)(?![\p{L}\p{N}])`,
  'iu',
)

// True when the note clearly asks for a CV without a photo.
export function wantsNoPhoto(note: string): boolean {
  return NO_PHOTO.test(note)
}

// True when the note asks for an ATS (applicant-tracking-friendly) design: those are the designs meant to have no photo, so it decides which designs the model is offered.
export function wantsAts(note: string): boolean {
  return /(?<![\p{L}\p{N}])ats(?![\p{L}\p{N}])/iu.test(note)
}

export interface PhotoDecision {
  attach: boolean
  // why: the CV has the photo ('photo'), the note asked for none ('asked-none'), or the profile has no photo ('no-profile-photo')
  reason: 'photo' | 'asked-none' | 'no-profile-photo'
}

export function decidePhoto(profileHasPhoto: boolean, note: string): PhotoDecision {
  if (wantsNoPhoto(note)) return { attach: false, reason: 'asked-none' }
  if (!profileHasPhoto) return { attach: false, reason: 'no-profile-photo' }
  return { attach: true, reason: 'photo' }
}
