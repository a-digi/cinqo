// Module-level indirection binding the bridge's openConversation() to
// GlobalChatWidget's own conversation-selection logic while it's
// mounted — same pattern toolNavigator.ts already established for
// navigate(): the bridge itself is built once at boot (loadTools),
// before any component with the real context/hook is mounted, so it
// can't call into the widget directly; the component that *does* have
// that access wires itself in later via setToolConversationOpener.
// See plan/ai/tools/career/step-60-generate-with-ai-live-chat-window.md.
let current: ((conversationId: string) => void) | null = null

export function setToolConversationOpener(fn: ((conversationId: string) => void) | null): void {
  current = fn
}

export function openToolConversation(conversationId: string): void {
  current?.(conversationId)
}
