// conversationEngine.ts (plan 197, step 5) — tells the Browser tool which engine reads the pages of an AI conversation, once, when the conversation
// of a link that is set to Rod is created. The AI reads pages with fetch_page_html, a tool that has no link and no engine; under a Browser that
// has Katana selected that read would go through Katana's Chrome, the very browser the site stopped. The Browser remembers the answer for the
// conversation for an hour (nothing is stored, and the AI cannot change it). A link that follows the Browser's engine sends nothing.

const CONVERSATION_ENGINE_URL = '/api/v1/tools/browser/proxy/conversation-engine'

// bindConversationEngine returns the conversation unchanged. Best effort on purpose: an older Browser tool, or one that cannot be reached, only
// means the AI's reads follow the Browser's selected engine as before; the run itself is never stopped by it.
export async function bindConversationEngine<T extends { id: string }>(link: { crawlEngine?: string }, conversation: T): Promise<T> {
  if (link.crawlEngine !== 'rod') return conversation
  try {
    await fetch(CONVERSATION_ENGINE_URL, {
      method: 'PUT',
      credentials: 'include',
      body: JSON.stringify({ conversationId: conversation.id, engine: 'rod' }),
    })
  } catch {
    // nothing to do: the reads then follow the Browser's selected engine
  }
  return conversation
}
