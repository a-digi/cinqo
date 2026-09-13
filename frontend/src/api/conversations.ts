import { apiDelete, apiGet, apiPatch, apiPost } from './client'

// Already camelCase on the wire (api/src/conversation/handler's own
// response DTOs) — no snake_case *Raw mapping needed, same as
// api/platforms.ts.

// platformId/model are fixed at creation and never change afterward
// (plan/ai/conversation/step-07-fixed-platform-and-model-per-conversation.md).
export interface Conversation {
  id: string
  title: string
  startedAt: string
  platformId: string
  model: string
}

// No `id` field on a message — content lives in a file, not a database
// row (plan/ai/conversation/step-01); a message is identified by its
// own timestamp instead.
export interface ConversationMessage {
  role: 'user' | 'assistant'
  content: string
  createdAt: string
}

export interface ConversationDetail extends Conversation {
  messages: ConversationMessage[]
}

export async function fetchConversations(): Promise<Conversation[]> {
  const raw = await apiGet<{ message: Conversation[] }>('/api/v1/conversations')
  return raw.message
}

export async function createConversation(input: { title?: string; platformId: string; model?: string }): Promise<Conversation> {
  const raw = await apiPost<{ message: Conversation }>('/api/v1/conversations', input)
  return raw.message
}

export async function fetchConversation(id: string): Promise<ConversationDetail> {
  const raw = await apiGet<{ message: ConversationDetail }>(`/api/v1/conversations/${encodeURIComponent(id)}`)
  return raw.message
}

export async function renameConversation(id: string, title: string): Promise<Conversation> {
  const raw = await apiPatch<{ message: Conversation }>(`/api/v1/conversations/${encodeURIComponent(id)}`, { title })
  return raw.message
}

export async function deleteConversation(id: string): Promise<void> {
  await apiDelete(`/api/v1/conversations/${encodeURIComponent(id)}`)
}

// platformId/model no longer travel per-message — a conversation's own
// fixed values apply automatically (step 7).
export async function sendMessage(conversationId: string, input: { content: string }): Promise<ConversationMessage> {
  const raw = await apiPost<{ message: ConversationMessage }>(
    `/api/v1/conversations/${encodeURIComponent(conversationId)}/messages`,
    input,
  )
  return raw.message
}
