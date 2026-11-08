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

// The same import as a sequence of short messages, for a model that cannot follow the long one (plan 181, step 9; reworked in plan 208).
// A small local model given buildImportPrompt never called a tool (0 of 10 on a replay) and wrote the failure sentence the prompt
// prescribes instead. Phi-4-mini then failed the first short steps too: it was offered all ten tools in every step, so it invented one
// (cv_import_add_persona) and wrote its call as text. So every step now says only what that step needs, and nothing else:
//  - the tools of the step, named in the message and the only ones the step's conversation offers (IMPORT_STEP_TOOLS);
//  - the actions as a numbered list, in the order they are made, with the read of the CV first;
//  - one plain line to reply with when the last call has been answered, so the step ends in a way the page can see.
// Rules of a weak-model prompt, kept by every message below (a check script fails when one is broken):
//  - one action per message, and the rules of a part only in the message of that part;
//  - a step names exactly the tools it offers, by their exact names;
//  - the words of an example are the tool's own: its actions are "insert" and "skip" (a first version said "add", which the tool
//    refuses; a replay on a fake tool did not notice, the real run did).
export const IMPORT_STEP_COUNT = 9

interface ImportStep {
  title: string
  tools: readonly string[]
  // The numbered actions of the step; the last line of the message is added around them.
  body: (cvFileId: string, runId: string) => string
  // What ends the step. The default asks for "STEP n DONE"; the last step asks for one plain sentence.
  ending?: string
}

const NOT_A_CV = 'If the document is not a CV (a form, a letter, an invoice, a certificate...), call it with no entries instead.'
// An entry's shape only shows where the CV's own content goes (without it a small model copies the key names as values).
const SHAPE = "The text in angle brackets only shows where the CV's own content goes: replace it with the real content."
const DATES = 'Dates are exactly YYYY, YYYY-MM or YYYY-MM-DD (write "Sep 2010" as 2010-09); leave a date empty when the CV does not give it.'

const readCv = (cvFileId: string): string => `Call pdf_to_markdown with url="media:${cvFileId}". It returns the text of the CV.`

const IMPORT_STEPS: ImportStep[] = [
  {
    title: 'read the CV',
    tools: ['pdf_to_markdown'],
    body: (cv) => `1. ${readCv(cv)}`,
  },
  {
    title: 'look at what is already stored',
    tools: ['list_profiles', 'list_personas', 'get_persona_details', 'list_education'],
    body: () =>
      '1. Call list_profiles.\n2. Call list_personas.\n3. Call get_persona_details once for each persona that list_personas returned.\n4. Call list_education once for each profile that list_profiles returned.',
  },
  {
    title: 'the profile',
    tools: ['pdf_to_markdown', 'cv_import_set'],
    body: (cv, run) =>
      `1. ${readCv(cv)}\n2. Call cv_import_set with runId "${run}", section "profile" and this value:\n{"action":"insert","possibleDuplicateOf":null,"name":"<the person's name>"}\n${SHAPE} If the document is not a CV, use "skip" instead of "insert" and invent no person. Make the cv_import_set call once.`,
  },
  {
    title: 'the persona',
    tools: ['pdf_to_markdown', 'cv_import_set'],
    body: (cv, run) =>
      `1. ${readCv(cv)}\n2. Call cv_import_set with runId "${run}", section "persona" and this value (one persona only):\n{"action":"insert","possibleDuplicateOf":null,"name":"<a short name for the persona>","title":"<the job title>"}\n${SHAPE} If the document is not a CV, use "skip" instead of "insert". Make the cv_import_set call once.`,
  },
  {
    title: 'the personal details',
    tools: ['pdf_to_markdown', 'cv_import_set'],
    body: (cv, run) =>
      `1. ${readCv(cv)}\n2. Call cv_import_set with runId "${run}", section "personalDetails" and this value:\n{"action":"insert","possibleDuplicateOf":null,"headline":"<one line>","summary":"<two or three sentences>","location":"<city, country>"}\n${SHAPE} If the document is not a CV, use "skip" instead of "insert" and empty texts. Make the cv_import_set call once.`,
  },
  {
    title: 'the skills',
    tools: ['pdf_to_markdown', 'cv_import_add_skills'],
    body: (cv, run) =>
      `1. ${readCv(cv)}\n2. Call cv_import_add_skills with runId "${run}" and items: one entry for each skill the CV lists (at most 30), in this shape:\n[{"action":"insert","possibleDuplicateOf":null,"value":"<skill 1>"},{"action":"insert","possibleDuplicateOf":null,"value":"<skill 2>"}]\n${SHAPE} If the CV lists no skills, send an empty list. ${NOT_A_CV} Make the cv_import_add_skills call once.`,
  },
  {
    title: 'the work experience',
    tools: ['pdf_to_markdown', 'cv_import_add_experience'],
    body: (cv, run) =>
      `1. ${readCv(cv)}\n2. Count the jobs the CV lists (every position with its own header of company, title and dates) and the bullet points under each job.\n3. Call cv_import_add_experience with runId "${run}" and items: the jobs, newest first as the CV lists them, the oldest and the shortest one too, at most 3 per call (5 if they are short). A job has this shape:\n{"action":"insert","possibleDuplicateOf":null,"company":"<company>","title":"<job title>","startDate":"<YYYY-MM>","endDate":"<YYYY-MM, or empty for a current job>","bulletsInCv":<number of bullet points>,"description":"<description>"}\n${SHAPE} ${DATES}\nThe description holds the WHOLE text of the job, one line for every bullet point:\n<introduction sentence of the job>\n- <first bullet point of the job>\n- <second bullet point of the job>\nTechnologies: <the technology list of the job>\nKeep all numbers, names and tools; never drop a bullet point, never merge bullet points, never add anything the CV does not say. Keep the CV's language. Lines of a side column (skills, certificates, languages) are not part of a job.\n4. Compare the number of jobs in the tool's answer with the number you have sent so far, and call cv_import_add_experience again with the jobs still missing. If the tool answers with a warning about a job, send that job again with all of its text (the same company, title and start date replaces the earlier one).\nIf the CV lists no work experience, send an empty list. ${NOT_A_CV}`,
  },
  {
    title: 'the education',
    tools: ['pdf_to_markdown', 'cv_import_add_education'],
    body: (cv, run) =>
      `1. ${readCv(cv)}\n2. Look through the CV text for schools, universities, degrees, certificates and courses (not jobs), under headings such as Education, Ausbildung, Studium, Abschlüsse, Qualifikationen, Weiterbildung and Certifications.\n3. Call cv_import_add_education with runId "${run}" and items: every entry you found (at most 10 per call), each in this shape:\n{"action":"insert","possibleDuplicateOf":null,"institution":"<school>","degree":"<degree>","fieldOfStudy":"<field>","startDate":"<YYYY>","endDate":"<YYYY>","description":""}\n${SHAPE} ${DATES} Leave endDate empty for something still going on. A degree or certificate whose school the CV does not name is still education: send it with institution "Not stated" and never guess a school. Never invent education.\nIf the CV has none, send an empty list. ${NOT_A_CV} Make the cv_import_add_education call once.`,
  },
  {
    title: 'hand over the result',
    tools: ['cv_import_finish'],
    body: (_cv, run) => `1. Call cv_import_finish with runId "${run}" and jobsInCv set to the number of jobs the CV lists. Make the call once.`,
    ending:
      'After the call has been answered, write one short plain sentence saying the analysis is done, with no JSON and no markdown, and do not list the proposal. If the document is not a CV, say in that sentence that it does not look like a CV.',
  },
]

// The tools a step offers: the page creates the step's conversation with exactly these, so a model cannot pick (or invent) another one.
export const IMPORT_STEP_TOOLS: readonly (readonly string[])[] = IMPORT_STEPS.map((step) => step.tools)

export function buildImportSteps(cvFileId: string, runId: string): string[] {
  return IMPORT_STEPS.map((step, index) => {
    const n = String(index + 1)
    const ending = step.ending ?? `Only when every call above has been answered, reply with exactly this line and nothing else: STEP ${n} DONE`
    return `STEP ${n} OF ${String(IMPORT_STEP_COUNT)}: ${step.title}. You are importing a CV into a career profile, one small step at a time.

Tools you can use in this step: ${step.tools.join(', ')}. There are no other tools: never use or invent another one. Make each call as a real tool call, and never write a tool call as text.

${step.body(cvFileId, runId)}

${ending}`
  })
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

// --- plan 207: what each step must do, and how a step that did not is corrected -------------------------------------------------
//
// Only for the step-by-step path (a platform whose models need short steps). The tools a step called are known from the turn's log, so a
// step can be checked without reading the draft: a step that stores something must have read the CV first (a small model that did not
// store the example text, "<the person's name>", as the person's name), and a required step must have called its store tool.

export interface StepExpectation {
  // What the step is called in a message to the person.
  label: string
  // The step reads the CV with pdf_to_markdown first.
  read: boolean
  // The tool that stores the step's result, or null when the step stores nothing.
  store: string | null
  // The step must call its store tool. False for a part the CV may not have (skills, experience, education: "if the CV lists none, send none").
  required: boolean
}

// One entry for each message of buildImportSteps, in the same order.
export const IMPORT_STEP_EXPECTATIONS: StepExpectation[] = [
  { label: 'reading the CV', read: true, store: null, required: true },
  { label: 'looking at what is stored', read: false, store: null, required: false },
  { label: 'profile', read: true, store: 'cv_import_set', required: true },
  { label: 'persona', read: true, store: 'cv_import_set', required: true },
  { label: 'personal details', read: true, store: 'cv_import_set', required: true },
  { label: 'skills', read: true, store: 'cv_import_add_skills', required: false },
  { label: 'work experience', read: true, store: 'cv_import_add_experience', required: false },
  { label: 'education', read: true, store: 'cv_import_add_education', required: false },
  { label: 'result', read: false, store: 'cv_import_finish', required: true },
]

export type StepVerdict = 'ok' | 'no_read' | 'no_store'

// Whether a step did what it had to, from the tools its turn called in order. Unknown calls (an older core) are never held against a step.
export function checkStep(expectation: StepExpectation, toolsCalled: string[] | undefined): StepVerdict {
  if (toolsCalled === undefined) return 'ok'
  const readAt = toolsCalled.indexOf('pdf_to_markdown')
  const storeAt = expectation.store === null ? -1 : toolsCalled.indexOf(expectation.store)
  if (expectation.read) {
    if (readAt < 0 && (storeAt >= 0 || expectation.required || expectation.store !== null)) return 'no_read'
    if (storeAt >= 0 && readAt > storeAt) return 'no_read' // it stored first and read afterwards: what it stored was not from the CV
  }
  if (expectation.required && expectation.store !== null && storeAt < 0) return 'no_store'
  return 'ok'
}

// The message that asks a step to be done again, in the same conversation, when checkStep found a fault. The model that wrote the
// example text instead of reading the CV is told so, and to do the two calls in order.
export function buildStepCorrection(expectation: StepExpectation, verdict: Exclude<StepVerdict, 'ok'>, cvFileId: string, runId: string): string {
  const read = `First call pdf_to_markdown with url="media:${cvFileId}" and read the CV text it returns.`
  const store = expectation.store === null ? '' : ` Then call ${expectation.store} with runId "${runId}" and the real content from the CV text, exactly as the step above describes it.`
  if (verdict === 'no_read') {
    return `You did not read the CV in your last answer, so what you stored, if anything, is only the example text. Do the step again now, in this order. ${read}${store} This replaces what you stored before. Make each call once, as a real tool call (never written as text).`
  }
  return `You did not call ${expectation.store ?? 'the tool'} in your last answer, so nothing was stored for the ${expectation.label}. Do it now. ${expectation.read ? read : ''}${store} Make each call once, as a real tool call (never written as text).`
}
