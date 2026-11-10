import { AppScopes } from '../../config/security/scopes'

// Whoever may manage keys is pointed to the page where one is added; the others are only told there is none
// (plan/ai/conversation/step-229-platforms-with-a-key-only.md).
export function canAddKeys(scopes: string[]): boolean {
  return scopes.includes(AppScopes.SuperAdmin) || scopes.includes(AppScopes.PlatformManage)
}
