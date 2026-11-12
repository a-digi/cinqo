import { AppLink } from '../../Shared/AppLink/AppLink'
import { cardClassName, cardStyle } from '../../Shared/cardStyle'
import { LOCAL_AI_PATH, REMOTE_AI_PATH } from '../../Cinqo/Platform/modelLabels'
import { ACCENTS, gradient } from '../JobsPage/jobsTheme'

// What the Import CV page shows instead of the upload while no AI is set up: the import reads the CV with an AI, so the person has to set up either a remote
// AI (the API key of a cloud platform) or a local AI (a model on this computer) first. `onRecheck` reads the platforms and keys again.
export function ImportNeedsAi({ onRecheck }: { onRecheck: () => void }) {
  const remote = ACCENTS[4]
  const local = ACCENTS[1]
  const option = 'flex h-full flex-col rounded-2xl border bg-white p-4 shadow-sm'
  const linkClass = 'mt-auto inline-block self-start rounded-full px-4 py-1.5 text-sm font-semibold text-white shadow-sm hover:opacity-90'
  return (
    <section aria-labelledby="import-needs-ai-title" className={`career-enter career-delay-1 ${cardClassName}`} style={cardStyle}>
      <h2 id="import-needs-ai-title" className="text-base font-semibold text-gray-900">
        Set up an AI to import a CV
      </h2>
      <p className="mt-1 text-sm text-gray-600">
        Importing reads your CV with an AI and proposes a profile, target roles, skills, experience and education. No AI is set up yet, so
        the upload is switched off. Set up one of these, then come back:
      </p>
      <div className="mt-4 grid gap-3 md:grid-cols-2">
        <div className={option} style={{ borderColor: remote.from }} data-testid="needs-ai-remote">
          <h3 className="text-sm font-semibold" style={{ color: remote.text }}>
            Remote AI, with an API key
          </h3>
          <p className="my-2 text-sm text-gray-600">
            Use an AI platform from the internet. Add its API key on the Remote Platforms page. The text of your CV is sent to that platform
            to be read.
          </p>
          <AppLink href={REMOTE_AI_PATH} className={linkClass} style={{ background: gradient(remote) }}>
            Add an API key
          </AppLink>
        </div>
        <div className={option} style={{ borderColor: local.from }} data-testid="needs-ai-local">
          <h3 className="text-sm font-semibold" style={{ color: local.text }}>
            Local AI, on this computer
          </h3>
          <p className="my-2 text-sm text-gray-600">
            Download a model and switch it on on the Local AI page. Nothing leaves this computer.
          </p>
          <AppLink href={LOCAL_AI_PATH} className={linkClass} style={{ background: gradient(local) }}>
            Set up a local AI
          </AppLink>
        </div>
      </div>
      <p className="mt-4 text-xs text-gray-500">
        Done?{' '}
        <button type="button" onClick={onRecheck} className="font-medium text-blue-700 underline hover:text-blue-800">
          Check again
        </button>
      </p>
    </section>
  )
}
