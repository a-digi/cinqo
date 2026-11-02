// Pure rules of the description preview of the import review (plan/ai/tools/career/step-177-cv-import-experience-quality.md, step 3):
// the person must see how much text of a job would be saved, so a job cut to one sentence cannot go unnoticed. No React, so they can
// be script-checked.

// The non-empty lines of a description (the AI writes one line per bullet point).
export function descriptionLineCount(description: string): number {
  return description.split('\n').filter((line) => line.trim() !== '').length
}

// The characters of a description as people count them (a letter with an accent is one), without the blanks around it.
export function descriptionLength(description: string): number {
  return Array.from(description.trim()).length
}

// What the closed preview says: "No description" when there is none, else its size, e.g. "Details · 470 characters · 10 lines".
export function descriptionSummary(description: string): string {
  const chars = descriptionLength(description)
  if (chars === 0) return 'No description'
  const lines = descriptionLineCount(description)
  return `Details · ${String(chars)} characters · ${String(lines)} ${lines === 1 ? 'line' : 'lines'}`
}
