// buildImportPrompt.ts — the instruction the AI gets to analyse an uploaded CV (step 155). The CV is an opaque "media:<fileId>"
// reference (never a fetchable URL: conversation/chat.go's invokeToolCall resolves it server-side before pdf_to_markdown runs).
// The AI does NOT print its result: it hands the proposal over in small pieces (step 165) by calling cv_import_set,
// cv_import_add_skills, cv_import_add_experience and cv_import_finish with the run id, which keep it on the run the page recorded
// first, and its last message is one plain sentence. So the chat shows an instruction and a
// "done", never a JSON object (the proposal's shape lives in the tool's own argument schema, not in this text). See
// plan/ai/tools/career/step-155-cv-import-result-by-tool-not-chat.md and plan/ai/tools/career/import-cv/step-05-ai-analysis-kickoff-and-polling.md.

// The tools this prompt asks the AI to use: keep the text below and this list in step (the check script fails when the prompt
// names a tool that is not in the list, or the list has a tool the prompt never names).
export const IMPORT_RUN_TOOLS = [
  'pdf_to_markdown',
  'list_profiles',
  'list_personas',
  'get_persona_details',
  'cv_import_set',
  'cv_import_add_skills',
  'cv_import_add_experience',
  'cv_import_finish',
] as const

export function buildImportPrompt(cvFileId: string, runId: string): string {
  return `You are helping import a CV into this user's career profile.

1. Call pdf_to_markdown with url="media:${cvFileId}" to read the CV's text content. If it fails (for example "tool unavailable") or returns no text, call no other tool, propose nothing and reply only "Could not read the CV: " followed by what the tool said.
2. Call list_profiles, list_personas, and get_persona_details (for each existing persona) to see what's already stored, so you can flag possible duplicates.
3. Based on the CV text and the existing data, decide what to propose for Profile, Persona, Personal Details, Skills, and Experience.
4. Hand the result over in SMALL pieces with runId ${runId}, never as one big call, and do not print it anywhere else: cv_import_set once each for section "profile", "persona" and "personalDetails" (value is an object); cv_import_add_skills with at most 30 skills per call; cv_import_add_experience with at most 5 jobs per call; then cv_import_finish once. If a tool answers with an error, correct only the piece it names and send that piece again.
5. Then reply with ONE short plain sentence saying the analysis is done, for example "Done: the CV is analysed and the proposal is ready for review." Do not repeat, summarise or list the proposal, and use no JSON and no markdown.

If you did read text and the document is not a CV (a form, a letter, an invoice, a certificate…), do not invent a person or a career: send profile, persona and personalDetails with action "skip" and empty texts, send no skills and no jobs, call cv_import_finish, and make your last sentence say that the document does not look like a CV.

Propose at most one persona for this run — do not propose several. Mark an item's action "skip" if it isn't relevant to this CV or you're not confident in it. Set possibleDuplicateOf when an item looks like it matches something already stored: for persona/experience use the existing item's own id; for a skill use the existing skill's own text; leave it null for profile/personalDetails and for anything with nothing to compare against.`
}

// The tool names a prompt mentions (snake_case words, in the order they first appear): the check script compares them with
// IMPORT_RUN_TOOLS both ways.
export function toolsNamedIn(prompt: string): string[] {
  const seen: string[] = []
  for (const name of prompt.match(/\b[a-z]+(?:_[a-z]+)+\b/g) ?? []) {
    if (!seen.includes(name)) seen.push(name)
  }
  return seen
}
