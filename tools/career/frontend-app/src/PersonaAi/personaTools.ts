// The tools a "create personas with AI" run may use (step 113): the AI only reads. The server is
// asked to restrict the run's conversation to exactly these (createConversation's allowedTools), so
// that it cannot create, change or delete anything whatever it is told or shown; the personas are
// created only by the user's own Apply. buildPersonaPrompt.ts names the tools the run needs: keep
// the two in step (personaTools' own check script fails when the prompt names a tool outside this
// list).

export const PERSONA_RUN_TOOLS = ['list_profiles', 'list_personas', 'get_persona_details'] as const

// Whether the conversation the server created is restricted to exactly the tools asked for. A
// server that does not know allowedTools ignores the field and answers without it: then the run
// must not start, because the AI would be unrestricted.
export function restrictionHolds(echoed: string[] | undefined, wanted: readonly string[]): boolean {
  if (echoed === undefined) return false
  // The same set of tools: nothing more (the AI could write), nothing less (it could not read).
  const have = new Set(echoed)
  return have.size === new Set(wanted).size && wanted.every((t) => have.has(t))
}

export const RESTRICTION_NOT_APPLIED_MESSAGE =
  'This server could not limit the AI to read-only tools, so the run was not started. Update the Cinqo app and try again.'
