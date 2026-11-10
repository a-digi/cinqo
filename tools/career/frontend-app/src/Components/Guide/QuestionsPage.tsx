import { useState } from 'react'
import { LayersIcon } from '../../Shared/Icons/icons'
import { AccordionItem } from '../../Shared/Accordion/AccordionItem'
import { AppLink } from '../../Shared/AppLink/AppLink'
import { useAddressSearch } from '../../Shared/AddressState/useAddressSearch'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { cardStyle } from '../../Shared/cardStyle'
import { FAQ_PATH, GUIDE_CONTACTS_PATH, GUIDE_JOBS_PATH, GUIDE_PROFILE_PATH } from '../../paths'
import { helpEntryForKey, questionsForPage } from './helpLinks'
import { QUESTIONS } from './questionsData'
import { toggleOpen } from './questionsRules'

const GUIDE_NAMES: Readonly<Record<string, string>> = {
  [GUIDE_PROFILE_PATH]: 'Profile & CV',
  [GUIDE_JOBS_PATH]: 'Finding jobs',
  [GUIDE_CONTACTS_PATH]: 'Companies & recruiters',
}

// The Questions page (plan/ai/tools/career/step-170-onboarding-guide.md, step 6): the common questions as accordions, one answer open at a
// time. With ?page=<key> (the Help link of a page, plan 239) only the questions about that page show, and the page's guide is offered.
// Static text, fetches nothing.
export function QuestionsPage() {
  const [openId, setOpenId] = useState<string | null>(null)
  const { search } = useAddressSearch(FAQ_PATH)
  const key = new URLSearchParams(search).get('page')
  const questions = questionsForPage(QUESTIONS, key)
  const guide = helpEntryForKey(key)?.entry.guide
  const filtered = questions.length !== QUESTIONS.length
  return (
    <div className="career-v-blur career-a-float career-t-wave max-w-3xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<LayersIcon />}
        title="Questions"
        description="The questions people ask most about the Career tool, with short answers. Each answer opens the page it is about."
        pill={{ text: `${String(questions.length)} ${questions.length === 1 ? 'question' : 'questions'}` }}
      />
      {filtered && (
        <p className="mb-4 flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-gray-600">
          <span>Showing the questions about this page.</span>
          {guide !== undefined && (
            <AppLink href={guide} className="font-medium text-blue-700 hover:underline">
              Read the guide: {GUIDE_NAMES[guide] ?? 'Guide'} →
            </AppLink>
          )}
          <AppLink href={FAQ_PATH} className="font-medium text-blue-700 hover:underline">
            All questions
          </AppLink>
        </p>
      )}
      <div className="career-stagger space-y-3">
        {questions.map((q) => (
          <AccordionItem
            key={q.id}
            variant="portal"
            style={cardStyle}
            isOpen={openId === q.id}
            onToggle={() => {
              setOpenId((current) => toggleOpen(current, q.id))
            }}
            header={<span className="text-sm font-semibold text-gray-900">{q.question}</span>}
          >
            <p className="text-sm leading-relaxed text-gray-600">{q.answer}</p>
            {q.link !== undefined && (
              <AppLink href={q.link.path} className="mt-2 inline-block text-sm font-medium text-blue-700 hover:underline">
                {q.link.label} →
              </AppLink>
            )}
          </AccordionItem>
        ))}
      </div>
    </div>
  )
}
