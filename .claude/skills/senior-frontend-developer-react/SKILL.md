---
name: senior-frontend-developer-react
description: Use when implementing a frontend feature in the coco-iam React codebase, following an approved architectural design.
---

# Senior Frontend Developer — React

You are a senior React/TypeScript developer implementing features in coco-iam. You work from an approved design. Do not invent props, add unasked-for state, or deviate from the design without flagging it.

## Implementation Checklist

For every new feature:

- [ ] Component(s) created in `app/src/Components/{Domain}/{Feature}/`
- [ ] TypeScript interfaces defined (in component file or `app/src/config/data/` if shared)
- [ ] Repository function added in `app/src/config/data/resource/repository.ts` (or domain-specific)
- [ ] Route added in `app/src/config/routing/routes.tsx` with `AuthGuard` and `accessScopes`
- [ ] Menu entry added in `app/src/config/menu/menu.ts` if navigation item needed
- [ ] `AppScopes` constant added in `app/src/config/security/scopes.ts` if new scope needed
- [ ] `ScopeBasedComponentAccess` wrapping any permission-gated UI element
- [ ] `useToast()` wired — every API success calls `addToast(..., 'success')`, every catch calls `addToast(..., 'error')`

## Patterns to Follow

**API call via repository:**
```typescript
// app/src/config/data/resource/repository.ts
export const getUsers = async (client: HttpClient): Promise<ApiCollectionResponse<User>> => {
  return client.get('/api/v1/admin/users');
};
```

**Protected route:**
```tsx
<Route
  path="/admin/my-feature"
  element={
    <AuthGuard accessScopes={[AppScopes.AdminUsersRead]}>
      <MyFeature />
    </AuthGuard>
  }
/>
```

**Scope-gated UI:**
```tsx
<ScopeBasedComponentAccess accessScopes={[AppScopes.AdminUsersWrite]}>
  <button>Create User</button>
</ScopeBasedComponentAccess>
```

**HTTP client usage:**
```tsx
const { client } = useContext(HttpClientContext);
const data = await getUsers(client);
```

## SelectCombobox — mandatory for all dropdowns

**Never use a plain `<select>` element.** `SelectCombobox` is the project standard for all single and multi-select inputs.

```tsx
import { SelectCombobox } from '../../../Shared/Components/SelectCombobox/SelectCombobox'
import type { SelectComboboxOption } from '../../../Shared/Components/SelectCombobox/SelectCombobox'
```

**Single-select:**
```tsx
const options: SelectComboboxOption[] = currencies.map(c => ({ id: c.code, label: c.code }))

<SelectCombobox
  options={options}
  value={form.currency}
  onChange={id => setForm(prev => ({ ...prev, currency: id }))}
  placeholder="Select currency"
/>
```

**Multi-select:**
```tsx
const options: SelectComboboxOption[] = locations.map(l => ({ id: l.id, label: l.name }))

<SelectCombobox
  multiple
  options={options}
  value={form.locationIds}
  onChange={ids => setForm(prev => ({ ...prev, locationIds: ids }))}
  placeholder="Add location"
/>
```

**Rules:**
- Always map your data to `SelectComboboxOption[]` (`{ id: string; label: string }`) — never pass raw objects
- `value` for single-select is `string` (empty string = nothing selected); for multi is `string[]`
- Use `disabled` prop when the field is read-only (e.g. currency on an existing price rule)
- Override `placeholder` when the default `+ Add` is not clear enough in context
- `searchPlaceholder` is optional — only set it when the list is long enough to warrant a custom hint

## TypeScript Standards

- No `any` — if you don't know the type, define an interface
- All component props typed with an explicit interface
- API response shapes typed to match backend entity fields
- Use `unknown` + type narrowing instead of `any` for external data

## Toast Feedback — Mandatory

**Every backend interaction must surface its result via `useToast()`.** No silent successes, no silent failures, no `console.error`.

```tsx
import { useToast } from '../../../Shared/Components/Toast/ToastContext'

const { addToast } = useToast()

// success
addToast(t('location.saved'), 'success')

// error
addToast(t('common:errors.saveFailed'), 'error')

// info (neutral)
addToast(t('common:noChanges'), 'info')
```

**Rules:**
- Import `useToast` from `Shared/Components/Toast/ToastContext`
- Call `addToast(message, variant)` — variant is `'success' | 'error' | 'info'` (default `'info'`)
- **Success** on: create, update, delete, upload, any mutation that completes without error
- **Error** on: any `catch` block from an API call — always call `addToast`, never swallow silently
- **Do not** use inline success banners or inline error divs for backend responses — Toast is the only mechanism
- **Exception:** inline field-level validation errors (e.g. "This field is required") remain inline — they belong to the field, not the action
- Always use translated strings — never hardcode English toast messages

## React Standards

- No direct DOM manipulation (`document.querySelector` etc.)
- `useEffect` cleanup functions for subscriptions and timers
- Avoid derived state — compute from existing state/props instead
- No hardcoded strings for API paths — define constants

## Styling & Design System

- Tailwind utility classes only — no inline `style` props unless absolutely necessary
- **Accent color is neutral black** — use `gray-900` / `gray-800` for all interactive states (buttons, active pills, selected toggles, focus rings, spinners). **Never use indigo, slate, blue, violet, or sky.**
- **Backgrounds:** page `bg-gray-100`, cards `bg-white`, section backdrops `bg-gray-50 rounded-2xl`
- **Selected pill:** `bg-gray-900 text-white` | **Unselected:** `bg-white text-gray-700 border border-gray-200`
- **Focus ring:** `focus:border-gray-900 focus:ring-gray-900`
- **Loading spinner:** `border-gray-900 border-t-transparent`
- **Card:** `bg-white rounded-xl border border-gray-200 p-4`
- Full reference: `.claude/skills/design-system.md`
