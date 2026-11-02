import { TimelineIcon } from '../../Shared/Icons/icons'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { cardClassName, cardStyle } from '../../Shared/cardStyle'
import { Timeline } from '../../Shared/Timeline/Timeline'
import { designLayout } from '../../Shared/Timeline/timelineRules'
import type { Guide } from './guideTypes'

// One guide of the Q&A menu (plan/ai/tools/career/step-170-onboarding-guide.md): a header card and the guide's steps on the shared
// Timeline. The design decides the page: the zigzag sits in one framed card on the narrow page, the two-sided designs use the wide page
// with the cards straight on it. Every guide page is this component with its own data (guideData.ts); it fetches nothing.
export function GuidePage({ guide }: { guide: Guide }) {
  const layout = designLayout(guide.design)
  return (
    <div className={`career-v-blur career-a-float career-t-wave ${layout.wide ? 'max-w-6xl' : 'max-w-3xl'} p-6 font-sans text-gray-900`}>
      <PageHeaderCard
        icon={<TimelineIcon />}
        title={guide.title}
        description={guide.description}
        pill={{ text: guide.steps.length === 1 ? '1 step' : `${guide.steps.length} steps` }}
      />
      {layout.framed ? (
        <section className={`${cardClassName} career-enter career-delay-1`} style={cardStyle}>
          <Timeline steps={guide.steps} ariaLabel={guide.title} design={guide.design} />
        </section>
      ) : (
        <Timeline steps={guide.steps} ariaLabel={guide.title} design={guide.design} />
      )}
    </div>
  )
}
