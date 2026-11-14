import { HOW_TO_CONTENT } from '../../Components/Admin/HowTo/HowToContent'
import { isKnownPageId } from '../../Components/Admin/HowTo/howToRules'
import { READER_PAGE_PATH } from '../../Components/FileReader/readerPageRules'
import { UNEXPECTED_STOPS_PATH } from '../routing/systemPaths'
import type { BreadcrumbRegistration } from './breadcrumbRules'

// The trails of the shell's own pages (plan/ai/frontend/step-270-breadcrumb.md, step 4): the pages that sit below another page. A flat
// top-level page (a list the menu opens) has none, because a one-item trail adds nothing; a page with no entry shows no bar at all.
// The words are the menu's own (config/menu/menu.ts); a menu GROUP has no page, so it is plain text in the trail. The addresses are
// the router's (config/routing/routes.tsx): a test fails when one of them stops being a real route.
export const SYSTEM_BREADCRUMBS: readonly BreadcrumbRegistration[] = [
  {
    path: `${UNEXPECTED_STOPS_PATH}/:id`,
    items: [{ label: 'System' }, { label: 'Unexpected stops', path: UNEXPECTED_STOPS_PATH }, { label: 'Stop' }],
  },
  {
    path: '/admin/tool-repositories/:id',
    items: [{ label: 'Tools' }, { label: 'Repositories', path: '/admin/tool-repositories' }, { label: 'Repository' }],
  },
  {
    // the list and one conversation share /conversations/:id?, so the list itself (/conversations) has no trail
    path: '/conversations/:id',
    items: [{ label: 'Conversations', path: '/conversations' }, { label: 'Conversation' }],
  },
  {
    path: '/conversations/:id/logs',
    items: ({ params }) => [
      { label: 'Conversations', path: '/conversations' },
      { label: 'Conversation', path: `/conversations/${encodeURIComponent(params.id)}` },
      { label: 'Logs' },
    ],
  },
  {
    path: '/admin/ai/how-to/:pageId',
    items: ({ params }) => [
      { label: 'AI' },
      { label: 'How To' },
      { label: isKnownPageId(params.pageId) ? HOW_TO_CONTENT[params.pageId].label : 'Platform' },
    ],
  },
  {
    // the reader has its own return link; the trail adds the way back in the browser history (the Back button)
    path: READER_PAGE_PATH,
    items: [{ label: 'Log reader' }],
  },
]
