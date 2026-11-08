# Loccando Admin — Design System

## Color Palette

### Backgrounds
| Role | Class |
|---|---|
| Page / layout | `bg-gray-100` |
| Card surface | `bg-white` |
| Section backdrop | `bg-gray-50` |
| Modal / dropdown | `bg-white` + `shadow-lg` |

### Text
| Role | Class |
|---|---|
| Primary | `text-gray-900` |
| Secondary | `text-gray-600` |
| Muted / labels / captions | `text-gray-500` |
| Placeholder | `text-gray-400` |
| Inverse (on dark bg) | `text-white` |

### Accent — Neutral Black (replaces all indigo/slate/blue)
**Never use indigo, slate, blue, violet, or sky for interactive or brand elements.**

| Role | Class |
|---|---|
| Primary button bg | `bg-gray-900` |
| Primary button hover | `hover:bg-gray-800` |
| Primary button text | `text-white` |
| Active / selected pill bg | `bg-gray-900` |
| Active / selected pill text | `text-white` |
| Selected toggle / chip | `bg-gray-900 text-white border-gray-900` |
| Link / subtle action text | `text-gray-700` |
| Link / action hover | `hover:text-gray-900` |
| Focus ring | `focus:ring-gray-900 focus:border-gray-900` |
| Icon button hover | `hover:bg-gray-100 hover:text-gray-900` |

### Selected / Unselected pill pattern
```
Selected:   bg-gray-900 text-white
Unselected: bg-white text-gray-700 border border-gray-200 hover:bg-gray-50
```

### Inline pill / tag (read-only label)
```
bg-gray-100 text-gray-800 border border-gray-200 rounded-full
```

### Borders
| Role | Class |
|---|---|
| Default card border | `border-gray-200` |
| Subtle divider | `border-gray-100` |
| Dashed section | `border-dashed border-gray-300` |
| Input default | `border-gray-300` |
| Input focused | `focus:border-gray-900` |

### Status Colors (semantic — do not replace)
| Role | Class |
|---|---|
| Error bg | `bg-red-50 text-red-700` |
| Success badge | `bg-green-100 text-green-700` |
| Inactive badge | `bg-gray-100 text-gray-500` |
| Warning | `bg-amber-50 text-amber-700` |

### Loading spinners
```
border-gray-900 border-t-transparent   (replaces border-indigo-600)
```

## Typography

| Role | Class |
|---|---|
| Page title | `text-2xl font-semibold text-gray-900` |
| Section title | `text-lg font-medium text-gray-900` |
| Card / group heading | `text-sm font-semibold text-gray-700` |
| Sub-label / caption | `text-xs text-gray-500` |
| Body | `text-sm text-gray-600` |

## Spacing & Shape

| Element | Class |
|---|---|
| Page wrapper | `space-y-8` |
| Section separator | `border-t border-gray-100 pt-6` |
| Card | `bg-white rounded-xl border border-gray-200 p-4` |
| Section backdrop | `bg-gray-50 rounded-2xl p-5` |
| Input | `rounded-md border border-gray-300 px-3 py-1.5 text-sm focus:border-gray-900 focus:outline-none focus:ring-1 focus:ring-gray-900` |
| Pill (toggle) | `inline-flex items-center px-3 py-1 rounded-full text-sm border transition-colors` |

## What is Banned

- `indigo-*` anywhere in UI (pills, buttons, focus, spinners, backgrounds, borders)
- `slate-*` for brand/interactive use
- `blue-*` for brand/interactive use
- Inline `style` props for color
- Hardcoded hex values

## Existing components to keep in mind

`SelectCombobox` selected pills use `bg-indigo-50 text-indigo-700 border-indigo-200` — this must be updated to `bg-gray-100 text-gray-800 border-gray-200` when touched.
