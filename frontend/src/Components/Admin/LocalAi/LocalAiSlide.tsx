import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { ApiError } from '../../../api/client'
import { DownloadProgress } from '../../../Shared/Components/Download/DownloadProgress'
import { useDownload } from '../../../Shared/Components/Download/useDownload'
import { Eyebrow, Frame, In, Subtitle, Title } from '../Platforms/PlatformSlides'
import { fetchLocalAi, type LocalAiResponse } from './localAiApi'
import { formatBytes } from '../../../Shared/Components/Download/format'
import { fitKind, fitLabel, runningDownloadIds } from './localAiFormat'

// The live progress of one running download, on the dark slide.
function SlideDownload({ id }: { id: string }) {
  const { download, error } = useDownload(id)
  return <DownloadProgress download={download} error={error} tone="dark" />
}

const FIT_TEXT = { fits: 'text-emerald-200', tight: 'text-amber-200', unknown: 'text-white/60' } as const

// The Local AI slide of the Platforms page. It reads the same endpoint as the settings page, so what it says (on or off,
// which models fit this computer) is always the real state. Nothing here changes a setting: the button opens the page.
export function LocalAiSlide() {
  const navigate = useNavigate()
  const [data, setData] = useState<LocalAiResponse | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    fetchLocalAi()
      .then(setData)
      .catch((err: unknown) => {
        setError(err instanceof ApiError ? err.message : 'Could not load the local AI.')
      })
  }, [])

  const on = data?.settings.enabled === true && data.status.state === 'configured'

  return (
    <Frame from="#0d9488" to="#0369a1" motif="network">
      <div className="grid items-center gap-8 md:grid-cols-[minmax(0,1.15fr)_minmax(0,1fr)]">
        <div>
          <In i={0} className="flex flex-wrap items-center gap-2">
            <Eyebrow>On this computer</Eyebrow>
            <span className="rounded-full bg-emerald-400/20 px-2.5 py-0.5 text-xs font-medium text-emerald-100 ring-1 ring-emerald-300/40">
              No cost per use
            </span>
            {data && (
              <span
                className={`rounded-full px-2.5 py-0.5 text-xs font-medium ring-1 ${on ? 'bg-white/20 text-white ring-white/50' : 'bg-white/5 text-white/70 ring-white/20'}`}
              >
                {on ? 'On' : data.status.canEnable ? 'Off' : 'Coming soon'}
              </span>
            )}
          </In>
          <In i={1}>
            <Title>Your own AI, on your own machine.</Title>
          </In>
          <In i={2}>
            <Subtitle>
              A small language model answers your conversations without a cloud provider. Nothing is sent to one, and nothing is billed per
              use.
            </Subtitle>
          </In>
          <In i={3}>
            <p className="mt-3 max-w-xl text-sm leading-relaxed text-white/60">
              <span className="font-semibold text-white/80">Good for: </span>
              Private, simple questions and drafts. Expect slower and simpler answers than the cloud models, and no tools or sub-agents: the
              model only answers. It uses the memory and processor of the machine it runs on.
            </p>
          </In>
          <In i={4} className="mt-6 flex flex-wrap items-center gap-3">
            <button
              type="button"
              onClick={() => {
                void navigate('/admin/local-ai')
              }}
              className="rounded-full bg-white px-5 py-2.5 text-sm font-semibold text-gray-900 shadow-lg transition hover:bg-white/90"
            >
              Open Local AI settings
            </button>
          </In>
        </div>
        <In i={3}>
          <div className="rounded-2xl bg-white/10 p-5 ring-1 ring-white/15 backdrop-blur-md">
            <p className="text-[11px] font-semibold uppercase tracking-[0.18em] text-emerald-200">Models Cinqo will manage</p>
            {error ? (
              <p role="alert" className="mt-2 text-sm text-white/80">
                {error}
              </p>
            ) : !data ? (
              <p className="mt-2 text-sm text-white/60">Loading…</p>
            ) : (
              <ul className="mt-2 space-y-2">
                {data.catalog.map((m) => (
                  <li key={m.id} className="flex flex-wrap items-baseline justify-between gap-x-3 text-sm">
                    <span className="font-mono text-xs text-white/90">
                      {m.name} · {formatBytes(m.sizeBytes)}
                    </span>
                    <span className={`text-xs ${FIT_TEXT[fitKind(m.fits)]}`}>{fitLabel(m.fits)}</span>
                  </li>
                ))}
              </ul>
            )}
            {data &&
              runningDownloadIds(data).map((id) => (
                <div key={id} className="mt-4">
                  <SlideDownload id={id} />
                </div>
              ))}
            <p className="mt-4 text-sm leading-relaxed text-white/60">
              Cinqo downloads the model you choose and runs it for you: there is nothing to install or to set up yourself.
            </p>
          </div>
        </In>
      </div>
    </Frame>
  )
}
