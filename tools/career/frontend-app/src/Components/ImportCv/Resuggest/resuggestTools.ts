// The tools a Re-suggest chat of the CV import review may use (plan 268): it reads the suggestions it is about, changes them with update_cv_import_suggestions (the server keeps it
// inside its section or rows), re-reads the CV and looks at what is already stored to flag duplicates. The server is asked to restrict the chat's conversation to exactly these
// (createConversation's allowedTools), so it can create, change or delete nothing else whatever it is told or shown; nothing reaches the profile before the person's own Insert.
// buildResuggestPrompt.ts names the tools it needs: keep the two in step (a check fails when the prompt names a tool outside this list, or the list has one the prompt never names).
export const SECTION_CHAT_TOOLS = [
  'get_cv_import_suggestions',
  'update_cv_import_suggestions',
  'pdf_to_markdown',
  'list_profiles',
  'list_personas',
  'get_persona_details',
  'list_education',
] as const
