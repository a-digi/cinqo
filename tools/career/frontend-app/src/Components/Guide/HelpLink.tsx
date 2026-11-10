import { HelpIcon } from '../../Shared/Icons/icons'
import { AppLink } from '../../Shared/AppLink/AppLink'
import { QUESTIONS } from './questionsData'
import { helpHref } from './helpLinks'

// The Help link of a page (plan/ai/tools/career/step-239-qa-pages-as-links-in-their-pages.md): a question-mark icon and the word Help,
// opening the Questions about the page, or the guide that covers it. page is the page's own address (its *_PATH constant). Shows nothing
// when the page has no help. Static; fetches nothing.
export function HelpLink({ page, className = '' }: { page: string; className?: string }) {
  const href = helpHref(page, QUESTIONS)
  if (href === null) return null
  return (
    <AppLink href={href} className={`inline-flex items-center gap-1 text-sm font-medium text-blue-700 hover:underline ${className}`.trim()}>
      <HelpIcon />
      Help
    </AppLink>
  )
}
