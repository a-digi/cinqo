// buildImportPrompt.ts — the exact strict JSON contract the AI is
// instructed to end its turn with, and the initial message embedding
// the capability URL (step 2) verbatim. Step 6 parses this same
// contract — kept here, not redefined there, since the instruction and
// the parser must agree on one shape. See
// plan/ai/tools/career/import-cv/step-05-ai-analysis-kickoff-and-polling.md.
const SCHEMA = `{
  "profile": { "action": "insert" | "skip", "possibleDuplicateOf": "string | null", "name": "string" },
  "persona": { "action": "insert" | "skip", "possibleDuplicateOf": "string | null", "name": "string", "title": "string" },
  "personalDetails": { "action": "insert" | "skip", "possibleDuplicateOf": null, "headline": "string", "summary": "string", "location": "string", "desiredTitles": "string", "desiredLocations": "string", "minSalary": 0 },
  "skills": [ { "action": "insert" | "skip", "possibleDuplicateOf": "string | null", "value": "string" } ],
  "experience": [ { "action": "insert" | "skip", "possibleDuplicateOf": "string | null", "company": "string", "title": "string", "startDate": "string", "endDate": "string", "description": "string" } ]
}`

export function buildImportPrompt(cvURL: string): string {
  return `You are helping import a CV into this user's career profile.

1. Call pdf_to_markdown with url="${cvURL}" to read the CV's text content.
2. Call list_profiles, list_personas, and get_persona_details (for each existing persona) to see what's already stored, so you can flag possible duplicates.
3. Based on the CV text and the existing data, decide what to propose for Profile, Persona, Personal Details, Skills, and Experience.
4. Your FINAL reply must be ONLY a single JSON object, no other text, no markdown code fence, matching exactly this shape:
${SCHEMA}

Propose at most one persona for this run — do not propose several. Mark an item's action "skip" if it isn't relevant to this CV or you're not confident in it. Set possibleDuplicateOf when an item looks like it matches something already stored: for persona/experience use the existing item's own id; for a skill use the existing skill's own text; leave it null for profile/personalDetails and for anything with nothing to compare against.`
}
