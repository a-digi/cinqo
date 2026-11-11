// The tools an "Optimize with AI" run may use (plan 258, step 4). The server is asked to restrict the run's conversation to exactly these (createConversation's allowedTools): the
// AI reads the texts and hands improved texts over with suggest_optimization, which only STORES them for the person to approve or decline. It has no tool that writes a profile or a
// target role, so nothing it says or is shown can change one: only the person's own approval does. buildOptimizePrompt.ts names the tools the run needs: keep the two in step.
//
// The echo check (restrictionHolds) and its message are the persona run's, shared.

export const OPTIMIZE_RUN_TOOLS = ['get_optimize_subject', 'suggest_optimization'] as const

export { RESTRICTION_NOT_APPLIED_MESSAGE, restrictionHolds } from '../../PersonaAi/personaTools'
