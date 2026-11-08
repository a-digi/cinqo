// Pure rules for the photo choice in the CV wizard (step 111): which of the profile's photos a CV
// uses, and what is sent for it. No React, so they can be script-checked.

export interface CvPhoto {
  mediaFileId: string
  isDefault: boolean
}

// The photo the CV will use: the chosen one when it is still one of the profile's photos, else the
// profile's default (the first marked default, or the first listed), or '' with no photos.
export function effectiveChoice(photos: CvPhoto[], chosen: string): string {
  if (photos.length === 0) return ''
  if (chosen !== '' && photos.some((p) => p.mediaFileId === chosen)) return chosen
  return (photos.find((p) => p.isDefault) ?? photos[0]).mediaFileId
}

// The picker only shows when the profile has more than one photo and the CV carries one.
export function showsPicker(attach: boolean, photos: CvPhoto[]): boolean {
  return attach && photos.length > 1
}

// What the preview and create requests name: only with several photos and the photo attached; with
// one photo (or none) nothing is named and the default is used, exactly as before.
export function photoToSend(attach: boolean, photos: CvPhoto[], chosen: string): string | undefined {
  return showsPicker(attach, photos) ? effectiveChoice(photos, chosen) : undefined
}
