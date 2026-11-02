import { useState } from 'react'
import { LayersIcon } from '../../Shared/Icons/icons'
import { AccordionItem } from '../../Shared/Accordion/AccordionItem'
import { AppLink } from '../../Shared/AppLink/AppLink'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { cardStyle } from '../../Shared/cardStyle'
import { QUESTIONS } from './questionsData'
import { toggleOpen } from './questionsRules'

// The Questions page of the Q&A menu (plan/ai/tools/career/step-170-onboarding-guide.md, step 6): the common questions as accordions,
// one answer open at a time. Static text, fetches nothing.
export function QuestionsPage() {
  const [openId, setOpenId] = useState<string | null>(null)
  return (
    <div className="career-v-blur career-a-float career-t-wave max-w-3xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<LayersIcon />}
        title="Questions"
        description="The questions people ask most about the Career tool, with short answers. Each answer opens the page it is about."
        pill={{ text: `${String(QUESTIONS.length)} questions` }}
      />
      <div className="career-stagger space-y-3">
        {QUESTIONS.map((q) => (
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
