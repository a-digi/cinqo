import { useCallback, useEffect, useState } from 'react'
import {
  assignUser,
  createOrganisation,
  fetchMembers,
  fetchOrganisations,
  setOrganisationBackground,
  updateOrganisation,
  type Organisation,
  type OrganisationMember,
} from '../../../api/organisations'
import { fetchAuthMe } from '../../../api/auth'
import { ApiError } from '../../../api/client'
import { LoadingSpinner } from '../../../Shared/Components/Loading/LoadingSpinner'
import { Pill } from '../../../Shared/Components/Pill/Pill'
import { Dropdown } from '../../../Shared/Components/Dropdown/Dropdown'
import { useConfirm } from '../../../Shared/Components/Modal/useConfirm'
import {
  canSuspend,
  flipWords,
  memberText,
  NAME_MAX,
  otherStatus,
  STATUS_WORDS,
  USER_ID_MAX,
  validName,
  validUserId,
} from './organisationRules'
import { switchedText, switchWords, turnOnQuestion } from './unattendedRules'
import { UnattendedSection } from './UnattendedSection'

const inputClass = 'rounded border border-gray-300 px-3 py-1.5 text-sm focus:border-gray-700 focus:outline-none'
const linkButton = 'text-xs font-medium text-gray-700 hover:text-gray-900 disabled:opacity-50'

const messageOf = (err: unknown, fallback: string): string => (err instanceof ApiError ? err.message : fallback)

// The operator's page for organisations (SaaS plan S1): create, rename, suspend and activate organisations, see who belongs to each, and move a user
// into another one. A user belongs to exactly one organisation. Core stores only the identity provider's user id, so members are shown by that id.
export function OrganisationsPage() {
  const [orgs, setOrgs] = useState<Organisation[] | null>(null)
  const [own, setOwn] = useState<{ id?: string; userId?: string }>({})
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [busy, setBusy] = useState('')
  const [editing, setEditing] = useState<{ id: string; name: string } | null>(null)
  const [open, setOpen] = useState('')
  const [members, setMembers] = useState<OrganisationMember[] | null>(null)
  const [overviewKey, setOverviewKey] = useState(0)
  const { confirm, dialog } = useConfirm()

  const load = useCallback(() => {
    fetchOrganisations()
      .then((list) => {
        setOrgs(list)
        setError('')
      })
      .catch((err: unknown) => {
        setError(messageOf(err, 'Failed to load organisations.'))
      })
  }, [])

  useEffect(() => {
    load()
    fetchAuthMe()
      .then((me) => {
        setOwn({ id: me.organisationId, userId: me.userId })
      })
      .catch(() => undefined)
  }, [load])

  // Runs one change and says whether it worked (a form only clears what was typed when it did).
  const run = async (key: string, job: () => Promise<void>, done: string): Promise<boolean> => {
    setBusy(key)
    setError('')
    setNotice('')
    try {
      await job()
      setNotice(done)
      return true
    } catch (err) {
      setError(messageOf(err, 'That did not work.'))
      return false
    } finally {
      setBusy('')
    }
  }

  const showMembers = (id: string) => {
    if (open === id) {
      setOpen('')
      return
    }
    setOpen(id)
    setMembers(null)
    fetchMembers(id)
      .then(setMembers)
      .catch((err: unknown) => {
        setError(messageOf(err, 'Failed to load the members.'))
      })
  }

  const flip = async (o: Organisation) => {
    const words = flipWords(o)
    if (!(await confirm({ title: words.title, message: words.message }))) return
    await run(
      `flip-${o.id}`,
      async () => {
        await updateOrganisation(o.id, { name: o.name, status: otherStatus(o.status) })
        load()
      },
      `"${o.name}" is now ${STATUS_WORDS[otherStatus(o.status)].toLowerCase()}.`,
    )
  }

  const toggleBackground = async (o: Organisation) => {
    const enable = !o.backgroundEnabled
    if (enable && !(await confirm({ title: 'Turn on unattended use', message: turnOnQuestion(o.name) }))) return
    await run(
      `bg-${o.id}`,
      async () => {
        const updated = await setOrganisationBackground(o.id, enable)
        setOrgs((prev) => (prev ? prev.map((x) => (x.id === updated.id ? updated : x)) : prev))
        setOverviewKey((k) => k + 1)
      },
      switchedText(o.name, enable),
    )
  }

  const rename = async () => {
    if (!editing) return
    const o = orgs?.find((x) => x.id === editing.id)
    if (!o || !validName(editing.name)) return
    await run(
      `rename-${o.id}`,
      async () => {
        await updateOrganisation(o.id, { name: editing.name.trim(), status: o.status })
        setEditing(null)
        load()
      },
      'Renamed.',
    )
  }

  if (!orgs) {
    return error ? (
      <p className="p-6 text-sm text-red-600">{error}</p>
    ) : (
      <div className="flex min-h-64 items-center justify-center">
        <LoadingSpinner label="Loading organisations…" />
      </div>
    )
  }

  return (
    <div className="cinqo-stagger cinqo-v-zoom space-y-6 p-6">
      <div className="max-w-4xl">
        <h1 className="text-xl font-semibold text-gray-900">Organisations</h1>
        <p className="mt-1 text-sm text-gray-500">
          Every user belongs to exactly one organisation, and a tool only ever shows a user their own organisation&apos;s data. A suspended
          organisation&apos;s users get no access to any tool. Users are shown by their id from the identity provider.
        </p>
      </div>

      {error && (
        <p role="alert" className="max-w-4xl rounded-md border border-red-200 bg-red-50 px-4 py-2 text-sm text-red-700">
          {error}
        </p>
      )}
      {notice && (
        <p role="status" className="max-w-4xl rounded-md border border-green-200 bg-green-50 px-4 py-2 text-sm text-green-800">
          {notice}
        </p>
      )}

      <div className="max-w-4xl rounded-lg border border-gray-200">
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="text-xs uppercase text-gray-400">
              <th className="px-4 py-2 font-medium">Name</th>
              <th className="px-4 py-2 font-medium">Status</th>
              <th className="px-4 py-2 font-medium">Users</th>
              <th className="px-4 py-2 font-medium">Unattended</th>
              <th className="px-4 py-2 font-medium" />
            </tr>
          </thead>
          <tbody>
            {orgs.map((o) => (
              <OrganisationRows
                key={o.id}
                org={o}
                isOwn={o.id === own.id}
                canSuspend={canSuspend(o, own.id)}
                busy={busy}
                editing={editing?.id === o.id ? editing.name : null}
                members={open === o.id ? members : undefined}
                ownUserId={own.userId}
                onEdit={(name) => {
                  setEditing(name === null ? null : { id: o.id, name })
                }}
                onSaveName={() => {
                  void rename()
                }}
                onFlip={() => {
                  void flip(o)
                }}
                onMembers={() => {
                  showMembers(o.id)
                }}
                onToggleBackground={() => {
                  void toggleBackground(o)
                }}
              />
            ))}
          </tbody>
        </table>
      </div>

      <UnattendedSection reloadKey={overviewKey} />
      <AddOrganisationForm
        onCreate={(name) =>
          run(
            'create',
            async () => {
              await createOrganisation(name)
              load()
            },
            `"${name}" was created.`,
          )
        }
        busy={busy === 'create'}
      />
      <AssignForm
        orgs={orgs}
        busy={busy === 'assign'}
        ownUserId={own.userId}
        onAssign={(userId, orgId) =>
          run(
            'assign',
            async () => {
              await assignUser(userId, orgId)
              setOpen('') // the lists of users that were open are stale now
              setMembers(null)
              load()
            },
            'The user was moved.',
          )
        }
      />
      {dialog}
    </div>
  )
}

function OrganisationRows(props: {
  org: Organisation
  isOwn: boolean
  canSuspend: boolean
  busy: string
  editing: string | null
  members: OrganisationMember[] | null | undefined
  ownUserId: string | undefined
  onEdit: (name: string | null) => void
  onSaveName: () => void
  onFlip: () => void
  onMembers: () => void
  onToggleBackground: () => void
}) {
  const { org, isOwn, busy, editing, members } = props
  const flipText = flipWords(org).button
  return (
    <>
      <tr className="border-t border-gray-100 align-top">
        <td className="px-4 py-2 text-gray-900">
          {editing === null ? (
            <>
              {org.name}
              {isOwn && <span className="ml-2 text-xs text-gray-400">(yours)</span>}
            </>
          ) : (
            <form
              className="flex items-center gap-2"
              onSubmit={(e) => {
                e.preventDefault()
                props.onSaveName()
              }}
            >
              <label className="sr-only" htmlFor={`rename-${org.id}`}>
                Name of {org.name}
              </label>
              <input
                id={`rename-${org.id}`}
                className={`${inputClass} w-56`}
                value={editing}
                maxLength={NAME_MAX * 2}
                onChange={(e) => {
                  props.onEdit(e.target.value)
                }}
              />
              <button type="submit" className={linkButton} disabled={!validName(editing) || busy === `rename-${org.id}`}>
                Save
              </button>
              <button
                type="button"
                className={linkButton}
                onClick={() => {
                  props.onEdit(null)
                }}
              >
                Cancel
              </button>
            </form>
          )}
        </td>
        <td className="px-4 py-2">
          <Pill variant={org.status === 'active' ? 'green' : 'red'}>{STATUS_WORDS[org.status]}</Pill>
        </td>
        <td className="px-4 py-2 text-gray-600">{memberText(org.memberCount)}</td>
        <td className="px-4 py-2">
          <div className="flex items-center gap-2">
            <Pill variant={org.backgroundEnabled ? 'green' : 'gray'}>{switchWords(org).state}</Pill>
            <button type="button" className={linkButton} disabled={busy === `bg-${org.id}`} onClick={props.onToggleBackground}>
              {switchWords(org).action}
            </button>
          </div>
        </td>
        <td className="px-4 py-2 text-right">
          <div className="flex justify-end gap-3">
            <button type="button" className={linkButton} onClick={props.onMembers} aria-expanded={members !== undefined}>
              {members === undefined ? 'Users' : 'Hide users'}
            </button>
            <button
              type="button"
              className={linkButton}
              onClick={() => {
                props.onEdit(org.name)
              }}
            >
              Rename
            </button>
            <button
              type="button"
              className="text-xs font-medium text-red-600 hover:text-red-800 disabled:opacity-40"
              disabled={!props.canSuspend || busy === `flip-${org.id}`}
              title={props.canSuspend ? undefined : 'You are in this organisation: suspending it would lock you out.'}
              onClick={props.onFlip}
            >
              {flipText}
            </button>
          </div>
        </td>
      </tr>
      {members !== undefined && (
        <tr className="border-t border-gray-50 bg-gray-50">
          <td colSpan={5} className="px-4 py-2">
            {members === null ? (
              <LoadingSpinner label="Loading users…" />
            ) : members.length === 0 ? (
              <p className="text-sm text-gray-400">No users yet. They join the Default organisation the first time they use a tool.</p>
            ) : (
              <ul className="space-y-0.5">
                {members.map((m) => (
                  <li key={m.userId} className="font-mono text-xs text-gray-700">
                    {m.userId}
                    {m.userId === props.ownUserId && <span className="ml-2 font-sans text-gray-400">(you)</span>}
                  </li>
                ))}
              </ul>
            )}
          </td>
        </tr>
      )}
    </>
  )
}

function AddOrganisationForm({ onCreate, busy }: { onCreate: (name: string) => Promise<boolean>; busy: boolean }) {
  const [name, setName] = useState('')
  return (
    <form
      className="max-w-4xl space-y-3 rounded-lg border border-gray-200 p-4"
      onSubmit={(e) => {
        e.preventDefault()
        if (!validName(name)) return
        void onCreate(name.trim()).then((ok) => {
          if (ok) setName('')
        })
      }}
    >
      <h2 className="text-sm font-semibold text-gray-900">New organisation</h2>
      <div className="flex flex-wrap gap-3">
        <label className="sr-only" htmlFor="new-organisation">
          Name of the new organisation
        </label>
        <input
          id="new-organisation"
          className={`${inputClass} min-w-48 flex-1`}
          placeholder="Name"
          value={name}
          onChange={(e) => {
            setName(e.target.value)
          }}
        />
        <button
          type="submit"
          disabled={busy || !validName(name)}
          className="rounded bg-gray-900 px-4 py-1.5 text-sm font-medium text-white disabled:opacity-50"
        >
          Create
        </button>
      </div>
    </form>
  )
}

function AssignForm({
  orgs,
  busy,
  ownUserId,
  onAssign,
}: {
  orgs: Organisation[]
  busy: boolean
  ownUserId: string | undefined
  onAssign: (userId: string, orgId: string) => Promise<boolean>
}) {
  const [userId, setUserId] = useState('')
  const [orgId, setOrgId] = useState('')
  return (
    <form
      className="max-w-4xl space-y-3 rounded-lg border border-gray-200 p-4"
      onSubmit={(e) => {
        e.preventDefault()
        if (!validUserId(userId) || !orgId) return
        void onAssign(userId.trim(), orgId).then((ok) => {
          if (ok) setUserId('')
        })
      }}
    >
      <h2 className="text-sm font-semibold text-gray-900">Move a user</h2>
      <p className="text-xs text-gray-500">
        Puts the user into this organisation; they leave the one they were in.
        {ownUserId ? (
          <>
            {' '}
            Your own id is <span className="font-mono">{ownUserId}</span>.
          </>
        ) : null}
      </p>
      <div className="flex flex-wrap gap-3">
        <label className="sr-only" htmlFor="assign-user">
          User id
        </label>
        <input
          id="assign-user"
          className={`${inputClass} min-w-64 flex-1 font-mono`}
          placeholder="User id"
          maxLength={USER_ID_MAX + 1}
          value={userId}
          onChange={(e) => {
            setUserId(e.target.value)
          }}
        />
        <div className="w-56">
          <Dropdown
            options={orgs.map((o) => ({ value: o.id, label: o.name }))}
            value={orgId}
            onChange={setOrgId}
            placeholder="Select organisation…"
          />
        </div>
        <button
          type="submit"
          disabled={busy || !validUserId(userId) || !orgId}
          className="rounded bg-gray-900 px-4 py-1.5 text-sm font-medium text-white disabled:opacity-50"
        >
          Move
        </button>
      </div>
    </form>
  )
}
