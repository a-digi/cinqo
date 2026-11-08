// The tools a "create a CV with AI" run may use (step 115). The server is asked to restrict the run's
// conversation to exactly these (createConversation's allowedTools), so the AI can read the persona,
// choose a template, render and save its CV, and nothing else (it cannot create, change or delete
// personas, jobs or anything). buildCvAiPrompt.ts names the tools the run needs: keep the two in step
// (cvAiRules' check script fails when the prompt names a tool outside this list).
//
// The echo check (restrictionHolds) and its message are the persona run's, shared.

export const CV_AI_RUN_TOOLS = [
  'get_persona_details',
  'list_cv_templates',
  'get_persona_cv_defaults',
  'render_cv_document',
  'generate_pdf',
  'save_persona_cv',
] as const

export { RESTRICTION_NOT_APPLIED_MESSAGE, restrictionHolds } from '../PersonaAi/personaTools'
