// loginGenerate.ts (plan 183, step 3) — the start of "Generate with AI" for a link that needs a login, as a sequence a test can run: the
// conversation exists first (its id is the login window's id), the window opens for the person, the run waits for "I am logged in", and only
// then is the attempt counted and the AI started. Anything that goes wrong BEFORE the start (the person cancels, the window cannot be opened,
// the conversation cannot be created) counts as nothing: no attempt, the window closed, the empty conversation removed.

// Thrown when the run ended before it started. error is undefined when the person cancelled.
export class LoginGenerateAbandoned extends Error {
  readonly reason: unknown
  constructor(reason: unknown) {
    super(reason instanceof Error ? reason.message : 'The run was not started.')
    this.name = 'LoginGenerateAbandoned'
    this.reason = reason
  }
}

export interface LoginGenerateSteps<C extends { id: string }> {
  create: () => Promise<C>
  // Opens the window for the conversation; rejects with the reason when the Browser refuses.
  open: (conversationId: string) => Promise<void>
  // Called once the window is open, so whoever owns the run can close it when the run ends.
  windowOpened: (conversationId: string) => void
  // Resolves when the person confirmed, rejects (LoginWaitCancelled) when they cancelled.
  waitForPerson: (conversationId: string) => Promise<void>
  // Counts the attempt; never rejects.
  recordStart: () => Promise<void>
  // Cleanup when the run never started.
  closeWindow: () => void
  removeConversation: (conversationId: string) => Promise<unknown>
  isCancel: (err: unknown) => boolean
}

// startAfterLogin resolves with the conversation once the person has logged in and the attempt is counted. It rejects with
// LoginGenerateAbandoned (reason: undefined for a cancel) after cleaning up when it could not get that far.
export async function startAfterLogin<C extends { id: string }>(steps: LoginGenerateSteps<C>): Promise<C> {
  let conversation: C
  try {
    conversation = await steps.create()
  } catch (err) {
    throw new LoginGenerateAbandoned(err)
  }
  try {
    await steps.open(conversation.id)
    steps.windowOpened(conversation.id)
    await steps.waitForPerson(conversation.id)
  } catch (err) {
    steps.closeWindow()
    await steps.removeConversation(conversation.id).catch(() => undefined)
    throw new LoginGenerateAbandoned(steps.isCancel(err) ? undefined : err)
  }
  await steps.recordStart()
  return conversation
}
