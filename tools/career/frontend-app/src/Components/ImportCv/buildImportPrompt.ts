// buildImportPrompt.ts — the instruction the AI gets to analyse an uploaded CV (step 155). The CV is an opaque "media:<fileId>"
// reference (never a fetchable URL: conversation/chat.go's invokeToolCall resolves it server-side before pdf_to_markdown runs).
// The AI does NOT print its result: it hands the proposal over in small pieces (step 165) by calling cv_import_set,
// cv_import_add_skills, cv_import_add_experience, cv_import_add_education (plan 175) and cv_import_finish with the run id, which keep it on the run the page recorded
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
  'list_education',
  'cv_import_set',
  'cv_import_add_skills',
  'cv_import_add_experience',
  'cv_import_add_education',
  'cv_import_finish',
] as const

export function buildImportPrompt(cvFileId: string, runId: string): string {
  return `You are helping import a CV into this user's career profile.

1. Call pdf_to_markdown with url="media:${cvFileId}" to read the CV's text content. If it fails (for example "tool unavailable") or returns no text, call no other tool, propose nothing and reply only "Could not read the CV: " followed by what the tool said.
2. Call list_profiles, list_personas, get_persona_details (for each existing persona) and list_education (for each existing profile) to see what's already stored, so you can flag possible duplicates.
3. Before you decide anything, read the work-experience section once and note for yourself two kinds of numbers: how many jobs the CV lists (every position with its own header of company, title and dates), and for each job how many bullet points are listed under it. You will send these numbers with the jobs and with cv_import_finish.
4. Based on the CV text and the existing data, decide what to propose for Profile, Persona, Personal Details, Skills, Experience, and Education.
5. Hand the result over in SMALL pieces with runId ${runId}, never as one big call, and do not print it anywhere else: cv_import_set once each for section "profile", "persona" and "personalDetails" (value is an object); cv_import_add_skills with at most 30 skills per call; cv_import_add_experience with at most 3 long jobs (or 5 short ones) per call, every job with its bulletsInCv, and after each call compare the number of jobs in the tool's answer with the jobs you have sent so far, until every job of the CV is sent; cv_import_add_education with at most 10 education entries per call, only when the CV has education (a CV without any: send none); then cv_import_finish once, with jobsInCv set to the number of jobs the CV lists. If a tool answers with an error, correct only the piece it names and send that piece again.
6. Then reply with ONE short plain sentence saying the analysis is done, for example "Done: the CV is analysed and the proposal is ready for review." Do not repeat, summarise or list the proposal, and use no JSON and no markdown.

If you did read text and the document is not a CV (a form, a letter, an invoice, a certificate…), do not invent a person or a career: send profile, persona and personalDetails with action "skip" and empty texts, send no skills, no jobs and no education, call cv_import_finish, and make your last sentence say that the document does not look like a CV.

Propose at most one persona for this run — do not propose several. Mark an item's action "skip" if it isn't relevant to this CV or you're not confident in it. Set possibleDuplicateOf when an item looks like it matches something already stored: for persona/experience use the existing item's own id; for a skill use the existing skill's own text; for education use the id of the matching entry from list_education; leave it null for profile/personalDetails and for anything with nothing to compare against.

Experience is the most important part to get right. Send EVERY job of the work-experience section, newest first as the CV lists them, including the oldest and the shortest one (the last job of a list is the one that gets forgotten). For each job send action, company, title, startDate and endDate (in the date shapes below; leave endDate empty for a current job), bulletsInCv, and a description that holds the WHOLE text of the job: the introduction sentence if the CV has one, then EVERY bullet point as its own line starting with "- ", then a last line "Technologies: " followed by the CV's technology list for that job. Keep all numbers, percentages, tools and names. You may improve the wording (fix typos and the broken line breaks of the PDF text, make a clumsy sentence read well, tighten a wordy bullet), but you must never cut: do not drop a bullet point, do not merge several bullet points into one sentence, do not reduce a job to its introduction, and do not add anything the CV does not say. Keep the CV's language. A description is about as long as the CV's text for that job (typically several hundred characters and up to 4000), never a single sentence unless that is all the CV says. The CV text may mix lines of a side column (skills, certificates, languages) into a job: those lines are not part of the job. Use this layout, with the CV's own content in place of the angle brackets:
<introduction sentence of the job>
- <first bullet point of the job, wording improved>
- <second bullet point of the job>
- <and so on, one line for every bullet point>
Technologies: <the technology list of the job>
If an answer carries a warning or an error about a description or a count, fix that job (with all of its text) and send it again: the same company, title and start date replaces the earlier one.

Education means schools, universities, degrees, certificates and courses (not jobs). For each entry send institution (required), degree, fieldOfStudy, startDate, endDate and a short description if the CV gives one. Dates must be exactly YYYY, YYYY-MM or YYYY-MM-DD (write "Sep 2010" as 2010-09); leave a date empty when the CV does not give it, and leave endDate empty for something still going on; the end is never before the start. Never invent education the CV does not state. A degree or certificate whose school the CV does not name is still education: send it with institution "Not stated" (never guess a school), and the user can fix it afterwards. Before you call cv_import_finish, look through the CV text once more for education (headings such as Education, Ausbildung, Studium, Abschlüsse, Qualifikationen, Weiterbildung, Certifications), send every entry you find with cv_import_add_education, and only then finish; send none only when the CV has no such section.`
}

// The same import as a sequence of short messages, for a model that cannot follow the long one (plan 181, step 9): a small local
// model given buildImportPrompt never called a tool (0 of 10 on a replay) and wrote the failure sentence the prompt prescribes
// instead; given one small action per message it called the right tool with valid arguments every time (8 of 8 per step).
// The page sends them one after the other in the same conversation and waits for each turn to end. Rules of a weak-model prompt,
// kept by every message below (a check script fails when one is broken):
//  - one action per message, and the rules of a part only in the message of that part;
//  - no quoted reply in a message that also asks for a tool call (the model writes the reply instead of calling the tool). The
//    one exception is the first message, whose reply is for a real tool error and comes after the call;
//  - the words of an example are the tool's own: its actions are "insert" and "skip" (a first version said "add", which the tool
//    refuses; a replay on a fake tool did not notice, the real run did);
//  - the tool is named by its exact name, and the first action is a command ("YOUR FIRST ACTION: call ...").
export const IMPORT_STEP_COUNT = 9

export function buildImportSteps(cvFileId: string, runId: string): string[] {
  const notACv = 'If the document is not a CV (a form, a letter, an invoice, a certificate...), send nothing in this step.'
  // Said in every step that stores something: a small model repeats a call it does not know is finished.
  const once = 'Make this call once. When the tool answers that it stored it, this step is done: write nothing more.'
  // The shape of an entry, to show where the CV's own content goes (without it a small model copies the key names as values).
  const shape = "These show the shape only: fill in the CV's own content."
  return [
    // 1 read
    `You are helping import a CV into this user's career profile. We do it one small step at a time.

YOUR FIRST ACTION: call the tool pdf_to_markdown with url="media:${cvFileId}". Do not write any text before you have its result. When you have the text, do nothing else until the next message.`,
    // 2 what is stored
    `Next step: look at what is already stored. Call list_profiles. Then call list_personas and get_persona_details for each existing persona, and list_education for each existing profile. This lets you flag possible duplicates later. Do nothing else in this step.`,
    // 3 profile
    `Next step: the profile. Using the CV text, call cv_import_set with runId "${runId}", section "profile" and value {"action":"insert","possibleDuplicateOf":null,"name":"<the person's name>"}. ${shape} Use "skip" instead of "insert" if the document is not a CV (do not invent a person) or you are not sure. If this person is already stored as a profile, set possibleDuplicateOf to that profile's id. ${once}`,
    // 4 persona
    `Next step: the persona. Using the CV text, call cv_import_set with runId "${runId}", section "persona" and value {"action":"insert","possibleDuplicateOf":null,"name":"<a short name for the persona>","title":"<the job title>"}. ${shape} Propose one persona only. Use "skip" instead of "insert" if the document is not a CV or you are not sure. If a persona it matches is already stored, set possibleDuplicateOf to that persona's id. ${once}`,
    // 5 personal details
    `Next step: the personal details. Using the CV text, call cv_import_set with runId "${runId}", section "personalDetails" and value {"action":"insert","possibleDuplicateOf":null,"headline":"<one line>","summary":"<two or three sentences>","location":"<city, country>"}. ${shape} Use "skip" instead of "insert" and empty texts if the document is not a CV. ${once}`,
    // 6 skills
    `Next step: the skills. Call cv_import_add_skills with runId "${runId}" and items holding one entry for each skill the CV lists, in this shape: [{"action":"insert","possibleDuplicateOf":null,"value":"<skill 1>"},{"action":"insert","possibleDuplicateOf":null,"value":"<skill 2>"}]. ${shape} At most 30 per call. If a skill is already stored, set possibleDuplicateOf to that stored skill's text. If the CV lists no skills, send none. ${notACv} ${once}`,
    // 7 experience
    `Next step: the work experience, the most important part. First count the jobs the CV lists (every position with its own header of company, title and dates) and the bullet points under each job. Then call cv_import_add_experience with runId "${runId}" and items holding the jobs, at most 3 per call (5 if they are short), until every job is sent, newest first as the CV lists them, the oldest and the shortest one too. A job has this shape: {"action":"insert","possibleDuplicateOf":null,"company":"<company>","title":"<job title>","startDate":"<YYYY-MM>","endDate":"<YYYY-MM, or empty for a current job>","bulletsInCv":<number of bullet points>,"description":"<description>"}. ${shape} Dates are exactly YYYY, YYYY-MM or YYYY-MM-DD.
The description holds the WHOLE text of the job in this layout, one line for every bullet point:
<introduction sentence of the job>
- <first bullet point of the job>
- <second bullet point of the job>
Technologies: <the technology list of the job>
Keep all numbers, names and tools. You may fix typos and broken line breaks, but never drop a bullet point, never merge several bullet points into one, and never add anything the CV does not say. Keep the CV's language. Lines of a side column (skills, certificates, languages) are not part of a job.
After each call compare the number of jobs in the tool's answer with the number you have sent so far, and send the jobs that are still missing. If the tool answers with a warning about a job, send that job again with all of its text: the same company, title and start date replaces the earlier one. If the CV lists no work experience, send none. ${notACv}`,
    // 8 education
    `Next step: the education. Education means schools, universities, degrees, certificates and courses (not jobs). Look through the CV text for headings such as Education, Ausbildung, Studium, Abschlüsse, Qualifikationen, Weiterbildung and Certifications. Call cv_import_add_education with runId "${runId}" and items holding every entry you find, at most 10 per call, each in this shape: {"action":"insert","possibleDuplicateOf":null,"institution":"<school>","degree":"<degree>","fieldOfStudy":"<field>","startDate":"<YYYY>","endDate":"<YYYY>","description":""}. ${shape} Dates are exactly YYYY, YYYY-MM or YYYY-MM-DD (write "Sep 2010" as 2010-09); leave a date empty when the CV does not give it, and leave endDate empty for something still going on. A degree or certificate whose school the CV does not name is still education: send it with institution "Not stated" and never guess a school. Set possibleDuplicateOf to the id of the matching entry from list_education. Never invent education. If the CV has none, send none. ${notACv} ${once}`,
    // 9 finish
    `Last step. Call cv_import_finish with runId "${runId}" and jobsInCv set to the number of jobs the CV lists. After that, write one short plain sentence saying the analysis is done. Do not repeat or list the proposal, and use no JSON and no markdown. If the document is not a CV, say in that sentence that it does not look like a CV.`,
  ]
}

// The follow-up message when an AI turn ended and the run still has no proposal (plan 165): a weak model stops after a refused piece,
// or writes a tool call as plain text ("<tool_call>cv_import_set…"), which does nothing. The draft lives on the run, so the same
// conversation simply continues; reason is the server's own answer (what is missing).
export function buildContinuePrompt(runId: string, reason: string): string {
  return `The import run ${runId} is still open and no proposal has been handed over yet (${reason.trim()}). Your last message did not hand over what is missing: either you wrote a tool call as text, which does nothing, or you stopped early. Continue now by calling the tools themselves, never by writing a tool call as text: send what is still missing with cv_import_set, cv_import_add_skills, cv_import_add_experience or cv_import_add_education (what was already accepted is kept; every job needs its whole text and its bulletsInCv), then call cv_import_finish with jobsInCv set to the number of jobs the CV lists. After that reply with ONE short plain sentence and no JSON.`
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
