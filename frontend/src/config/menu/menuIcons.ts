// The sidebar icons of the app's own menu items that need a glyph of their own (plan/ai/frontend/step-232-menu-icons.md). Raw SVG,
// drawn through Layout/SidebarIcon.tsx's allowlist sanitizer: only svg/path/circle/rect/line/... with the attributes it lists, at most 2000
// characters, so each is a plain monochrome outline on a 20x20 grid (stroke 1.3, currentColor) like the icons in menu.ts. Each says what
// its page does, with no brand logos. A check script (not part of the app) runs every icon of every menu through the same allowlist.
import type { HowToPageId } from '../../Components/Admin/HowTo/howToRules'

const OPEN = '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor">'

// Installed Tools: a box (a package that is installed).
export const installedToolsIcon = `${OPEN}<path d="M10 3l6 3v8l-6 3-6-3V6l6-3z" stroke-width="1.3" stroke-linejoin="round"/><path d="M4 6l6 3 6-3M10 9v8" stroke-width="1.3" stroke-linejoin="round" stroke-linecap="round"/></svg>`

// Repositories: a cloud with an arrow down (tools are downloaded from a repository).
export const repositoriesIcon = `${OPEN}<path d="M6 14a3.2 3.2 0 0 1-.4-6.3 4.3 4.3 0 0 1 8.3-.7A3.6 3.6 0 0 1 14 14" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/><path d="M10 9v7m-2.3-2.3L10 16l2.3-2.3" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/></svg>`

// Logs: a document with lines of text (a log to read).
export const logsIcon = `${OPEN}<path d="M5.5 3h6L15 6.5V16a1 1 0 0 1-1 1H5.5a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1z" stroke-width="1.3" stroke-linejoin="round"/><path d="M11.5 3v3.5H15M7 10h6M7 12.5h6M7 15h3.5" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/></svg>`

// Scopes: a list with check marks (what each role is allowed to do).
export const scopesIcon = `${OPEN}<path d="M3.5 5.7l1.3 1.3 2.2-2.5M3.5 10.7l1.3 1.3 2.2-2.5M3.5 15.7l1.3 1.3 2.2-2.5" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/><path d="M10 6h6.5M10 11h6.5M10 16h6.5" stroke-width="1.3" stroke-linecap="round"/></svg>`

// The How To pages, one glyph for what each page is about. The three providers are told apart by what they do, not by their logos.
export const howToIcons: Record<HowToPageId, string> = {
  // Overview: a circle with an "i".
  overview: `${OPEN}<circle cx="10" cy="10" r="7" stroke-width="1.3"/><path d="M10 9v4.5" stroke-width="1.3" stroke-linecap="round"/><circle cx="10" cy="6.6" r="0.9" fill="currentColor" stroke="none"/></svg>`,
  // Local Agents: a chip (a model that runs on this computer).
  'local-agents': `${OPEN}<rect x="5.5" y="5.5" width="9" height="9" rx="1.5" stroke-width="1.3"/><rect x="8" y="8" width="4" height="4" rx="0.6" stroke-width="1.3"/><path d="M8 3v2.5M12 3v2.5M8 14.5V17M12 14.5V17M3 8h2.5M3 12h2.5M14.5 8H17M14.5 12H17" stroke-width="1.3" stroke-linecap="round"/></svg>`,
  // OpenRouter: a route that branches (it sends you to many models).
  openrouter: `${OPEN}<circle cx="5" cy="10" r="1.7" stroke-width="1.3"/><circle cx="15" cy="5" r="1.7" stroke-width="1.3"/><circle cx="15" cy="15" r="1.7" stroke-width="1.3"/><path d="M6.6 9.2l6.8-3.4M6.6 10.8l6.8 3.4" stroke-width="1.3" stroke-linecap="round"/></svg>`,
  // OpenAI: a four-point sparkle.
  openai: `${OPEN}<path d="M10 3l1.7 4.6L16.4 9.3 11.7 11 10 16 8.3 11 3.6 9.3 8.3 7.6 10 3z" stroke-width="1.3" stroke-linejoin="round"/></svg>`,
  // Anthropic: a speech bubble with a spark in it.
  anthropic: `${OPEN}<path d="M4 4.5h12a1 1 0 0 1 1 1v7a1 1 0 0 1-1 1H8.5l-3 3v-3H4a1 1 0 0 1-1-1v-7a1 1 0 0 1 1-1z" stroke-width="1.3" stroke-linejoin="round"/><path d="M10 6.6l.8 1.7 1.7.8-1.7.8-.8 1.7-.8-1.7-1.7-.8 1.7-.8.8-1.7z" stroke-width="1" stroke-linejoin="round"/></svg>`,
}

// Unexpected stops: a warning triangle.
export const unexpectedStopsIcon = `${OPEN}<path d="M10 3.5l7 12.5H3L10 3.5z" stroke-width="1.3" stroke-linejoin="round"/><path d="M10 8.5v3.2M10 13.9v.1" stroke-width="1.5" stroke-linecap="round"/></svg>`
