import { useEffect, useMemo, useState } from 'react'
import {
  addApplicationMessage,
  deleteApplicationMessage,
  fetchAllCvDocuments,
  fetchApplicationMessages,
  fetchJobRecruiters,
  fetchProfiles,
  updateApplicationMessage,
  type ApplicationMessage,
  type ApplicationMessageInput,
  type CvDocument,
  type JobRecruiter,
  type Profile,
} from '../../api'
import { TimelineIcon } from '../../Shared/Icons/icons'
import { ConfirmationModal } from '../../Shared/ConfirmationModal/ConfirmationModal'
import { recruiterLabel } from '../Recruiters/recruiterMapRules'
import { profileLabel } from '../ProfileSwitcher/profileLabel'
import { InfoCard } from './InfoCard'
import { MessageModal } from './MessageModal'
import {
  channelText,
  counterpartText,
  directionLabel,
  emptyForm,
  entryCountText,
  formFromMessage,
  isPlanned,
  latestHappenedId,
  sortMessages,
  whenText,
  type MessageForm,
} from './protocolRules'

interface ProtocolCardProps {
  jobId: string
  // The profile the page is opened for ('' when it is opened for nobody in particular: the card then asks whose application it is).
  profileId: string
  profileName: string
}

const DIRECTION_STYLE: Record<ApplicationMessage['direction'], string> = {
  sent: 'bg-blue-50 text-blue-700 border-blue-200',
  received: 'bg-green-50 text-green-700 border-green-200',
  note: 'bg-gray-50 text-gray-600 border-gray-200',
}

type Editing = { kind: 'new'; form: MessageForm } | { kind: 'edit'; id: string; form: MessageForm }

// The "Protocol" card on a job's page (plan/ai/tools/career/step-176-application-protocol.md, step 3): the dated log of the communication
// of one application (one profile, one job) as a timeline, oldest first, the newest that really happened highlighted, a planned meeting
// marked. Entries are added, edited and deleted here; what is written is checked again by the server.
export function ProtocolCard({ jobId, profileId, profileName }: ProtocolCardProps) {
  const [profiles, setProfiles] = useState<Profile[] | null>(null)
  const [chosen, setChosen] = useState('')
  const effectiveProfile = profileId !== '' ? profileId : chosen
  const [messages, setMessages] = useState<ApplicationMessage[] | null>(null)
  const [recruiters, setRecruiters] = useState<JobRecruiter[]>([])
  const [cvDocuments, setCvDocuments] = useState<CvDocument[]>([])
  const [error, setError] = useState('')
  const [editing, setEditing] = useState<Editing | null>(null)
  const [deleting, setDeleting] = useState<ApplicationMessage | null>(null)
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(new Set())

  // Without a profile in the address: the profiles to choose from.
  useEffect(() => {
    if (profileId !== '') return
    let cancelled = false
    fetchProfiles()
      .then((list) => {
        if (cancelled) return
        setProfiles(list)
        setChosen((prev) => (prev !== '' || list.length !== 1 ? prev : list[0].id))
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err))
      })
    return () => {
      cancelled = true
    }
  }, [profileId])

  // The names the entries and the form need: the recruiters mapped to the job and the CVs (best effort: the log works without them).
  useEffect(() => {
    let cancelled = false
    fetchJobRecruiters(jobId)
      .then((list) => {
        if (!cancelled) setRecruiters(list ?? [])
      })
      .catch(() => undefined)
    fetchAllCvDocuments()
      .then((list) => {
        if (!cancelled) setCvDocuments(list)
      })
      .catch(() => undefined)
    return () => {
      cancelled = true
    }
  }, [jobId])

  useEffect(() => {
    setMessages(null)
    if (effectiveProfile === '') return
    let cancelled = false
    setError('')
    fetchApplicationMessages(effectiveProfile, jobId)
      .then((list) => {
        if (!cancelled) setMessages(list)
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err))
      })
    return () => {
      cancelled = true
    }
  }, [effectiveProfile, jobId])

  const now = new Date()
  const sorted = useMemo(() => sortMessages(messages ?? []), [messages])
  const latest = latestHappenedId(sorted, now)
  const recruiterName = (id: string): string => {
    const r = recruiters.find((x) => x.id === id)
    return r ? recruiterLabel(r) : ''
  }
  const cvTitle = (id: string): string => {
    if (id === '') return ''
    return cvDocuments.find((c) => c.id === id)?.title ?? 'a CV that no longer exists'
  }

  async function save(input: ApplicationMessageInput) {
    if (editing === null) return
    const list =
      editing.kind === 'new'
        ? await addApplicationMessage(effectiveProfile, jobId, input)
        : await updateApplicationMessage(editing.id, input)
    setMessages(list)
  }

  function confirmDelete() {
    if (deleting === null) return
    const target = deleting
    setDeleting(null)
    setError('')
    deleteApplicationMessage(target.id)
      .then(() => {
        setMessages((prev) => (prev ? prev.filter((m) => m.id !== target.id) : prev))
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  function toggle(id: string) {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const subtitle = profileId !== '' && profileName !== '' ? `of ${profileName}` : ''

  return (
    <InfoCard
      look="protocol"
      icon={<TimelineIcon className="h-4 w-4" />}
      title={subtitle === '' ? 'Protocol' : `Protocol ${subtitle}`}
      hint="The dated log of the messages, calls and meetings of this application."
    >
      {profileId === '' && (
        <label className="mb-3 block text-xs font-medium text-gray-500">
          Whose application
          <select
            value={chosen}
            onChange={(e) => {
              setChosen(e.target.value)
            }}
            disabled={profiles === null}
            className="mt-1 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
          >
            <option value="">{profiles === null ? 'Loading…' : profiles.length === 0 ? 'No profile yet' : 'Choose a profile'}</option>
            {profiles?.map((p) => (
              <option key={p.id} value={p.id}>
                {profileLabel(p)}
              </option>
            ))}
          </select>
        </label>
      )}

      {error !== '' && (
        <p role="alert" className="mb-3 text-xs text-red-700">
          {error}
        </p>
      )}
      {effectiveProfile !== '' && messages === null && error === '' && <p className="mb-3 text-xs text-gray-500">Loading…</p>}

      {messages !== null && (
        <>
          <p className="mb-3 text-xs text-gray-500">{entryCountText(messages.length)}</p>
          {sorted.length > 0 && (
            <ol className="mb-4 flex flex-col gap-2 border-l-2 border-gray-200 pl-4">
              {sorted.map((m) => {
                const planned = isPlanned(m, now)
                const who = counterpartText(m, recruiterName(m.recruiterId))
                const open = expanded.has(m.id)
                const long = m.body.length > 160 || m.body.includes('\n')
                return (
                  <li
                    key={m.id}
                    className={`relative rounded-md border bg-white/80 px-3 py-2 text-sm ${
                      m.id === latest ? 'border-gray-400 ring-1 ring-gray-300' : 'border-white'
                    }`}
                  >
                    <span
                      aria-hidden="true"
                      className={`absolute top-3 -left-[1.4rem] h-2.5 w-2.5 rounded-full border-2 border-white ${planned ? 'bg-amber-400' : 'bg-gray-400'}`}
                    />
                    <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-gray-500">
                      <span className={`rounded-full border px-2 py-0.5 font-medium ${DIRECTION_STYLE[m.direction]}`}>
                        {directionLabel(m.direction)}
                      </span>
                      <span>{channelText(m)}</span>
                      <span aria-hidden="true">·</span>
                      <span>{whenText(m)}</span>
                      {planned && <span className="rounded-full bg-amber-50 px-2 py-0.5 font-medium text-amber-700">Planned</span>}
                      {m.id === latest && <span className="rounded-full bg-gray-100 px-2 py-0.5 font-medium text-gray-700">Latest</span>}
                    </div>
                    {m.subject !== '' && <p className="mt-1 font-medium text-gray-900">{m.subject}</p>}
                    {who !== '' && (
                      <p className="text-xs text-gray-600">
                        {m.direction === 'received' ? 'From' : m.direction === 'sent' ? 'To' : 'With'} {who}
                      </p>
                    )}
                    {m.body !== '' && (
                      <p className={`mt-1 whitespace-pre-wrap text-gray-700 ${open || !long ? '' : 'line-clamp-3'}`}>{m.body}</p>
                    )}
                    {long && (
                      <button
                        type="button"
                        onClick={() => {
                          toggle(m.id)
                        }}
                        className="text-xs text-blue-700 underline hover:text-blue-800"
                      >
                        {open ? 'Show less' : 'Show all'}
                      </button>
                    )}
                    {(m.cvDocumentId !== '' || m.externalUrl !== '') && (
                      <p className="mt-1 flex flex-wrap gap-x-3 text-xs text-gray-600">
                        {m.cvDocumentId !== '' && <span>CV sent: {cvTitle(m.cvDocumentId)}</span>}
                        {m.externalUrl !== '' && (
                          <a href={m.externalUrl} target="_blank" rel="noreferrer" className="text-blue-700 underline hover:text-blue-800">
                            Open the thread
                          </a>
                        )}
                      </p>
                    )}
                    <div className="mt-1.5 flex gap-3 text-xs">
                      <button
                        type="button"
                        onClick={() => {
                          setEditing({ kind: 'edit', id: m.id, form: formFromMessage(m) })
                        }}
                        className="text-gray-500 underline hover:text-gray-700"
                      >
                        Edit
                      </button>
                      <button
                        type="button"
                        onClick={() => {
                          setDeleting(m)
                        }}
                        className="text-gray-500 underline hover:text-red-700"
                      >
                        Delete
                      </button>
                    </div>
                  </li>
                )
              })}
            </ol>
          )}
          <button
            type="button"
            onClick={() => {
              setEditing({ kind: 'new', form: emptyForm(new Date()) })
            }}
            className="rounded-md border border-gray-200 bg-white px-3 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-50"
          >
            Add entry
          </button>
        </>
      )}

      {editing !== null && (
        <MessageModal
          key={editing.kind === 'new' ? 'new' : editing.id}
          title={editing.kind === 'new' ? 'Add an entry to the protocol' : 'Edit the entry'}
          initial={editing.form}
          recruiters={recruiters}
          cvDocuments={cvDocuments}
          onSave={save}
          onClose={() => {
            setEditing(null)
          }}
        />
      )}
      <ConfirmationModal
        open={deleting !== null}
        title="Delete this entry?"
        message="The entry is removed from the protocol of this application. This cannot be undone."
        confirmLabel="Delete"
        variant="danger"
        onConfirm={confirmDelete}
        onCancel={() => {
          setDeleting(null)
        }}
      />
    </InfoCard>
  )
}
