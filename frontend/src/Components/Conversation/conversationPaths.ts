// conversationPaths.ts — the address of a conversation in the app (plan: a conversation opens at /conversations/<id>). Pure, so every branch can be
// script-checked. The id of a conversation is the lower-case UUID the server generates; anything else is not an id and never reaches the server.

export const CONVERSATIONS_PATH = '/conversations'

const CONVERSATION_ID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/

// Whether text is a conversation id: exactly a lower-case UUID. A typed or pasted address with capitals, spaces, a line break or anything else is
// refused rather than repaired, so what is sent to the server is always an id this app would have produced.
export function isConversationId(value: unknown): value is string {
  return typeof value === 'string' && CONVERSATION_ID.test(value)
}

// The address of one conversation, or of the list when there is no valid id (nothing selected, or text that is not an id): never a path built from
// text that could change where it leads (`../`, `?`, `#`).
export function conversationPath(id: string | null | undefined): string {
  return isConversationId(id) ? `${CONVERSATIONS_PATH}/${id}` : CONVERSATIONS_PATH
}
