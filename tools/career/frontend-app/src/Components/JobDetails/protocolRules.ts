// Pure rules of the Protocol card on a job's page (plan/ai/tools/career/step-176-application-protocol.md, step 3): the words for the
// directions and channels, how an entry's date reads, the entry as a form and back, the early check the server makes again, and which
// entry is the newest that really happened. No React, so they can be script-checked.
import type { ApplicationChannel, ApplicationDirection, ApplicationMessage, ApplicationMessageInput } from '../../api'

export const DIRECTIONS: readonly { value: ApplicationDirection; label: string }[] = [
  { value: 'sent', label: 'Sent' },
  { value: 'received', label: 'Received' },
  { value: 'note', label: 'Note' },
]

export const CHANNELS: readonly { value: ApplicationChannel; label: string }[] = [
  { value: 'email', label: 'E-mail' },
  { value: 'platform', label: 'Platform message' },
  { value: 'sms', label: 'SMS' },
  { value: 'whatsapp', label: 'WhatsApp' },
  { value: 'phone', label: 'Phone call' },
  { value: 'video', label: 'Video call' },
  { value: 'in_person', label: 'In person' },
  { value: 'letter', label: 'Letter' },
  { value: 'other', label: 'Other' },
]

// The limits the server checks (applicationlog.go), in characters.
export const MAX_SHORT_TEXT = 300
export const MAX_BODY = 20000
export const MAX_URL = 1000

export function directionLabel(d: ApplicationDirection): string {
  return DIRECTIONS.find((x) => x.value === d)?.label ?? d
}

// "E-mail", or "Platform message (LinkedIn)" when the person named the platform or app.
export function channelText(m: Pick<ApplicationMessage, 'channel' | 'channelLabel'>): string {
  const base = CHANNELS.find((x) => x.value === m.channel)?.label ?? m.channel
  const label = m.channelLabel.trim()
  return label === '' ? base : `${base} (${label})`
}

// The sort key of an entry: a day without a time counts as the start of that day (as the server orders it).
function key(m: Pick<ApplicationMessage, 'occurredAt' | 'hasTime'>): string {
  return m.hasTime ? m.occurredAt : `${m.occurredAt}T00:00:00Z`
}

// The entries oldest first (a conversation reads top to bottom); ties keep their order. A new array.
export function sortMessages(list: readonly ApplicationMessage[]): ApplicationMessage[] {
  return list
    .map((m, i) => ({ m, i }))
    .sort((a, b) => (key(a.m) < key(b.m) ? -1 : key(a.m) > key(b.m) ? 1 : a.i - b.i))
    .map((x) => x.m)
}

function pad(n: number): string {
  return String(n).padStart(2, '0')
}

// A date "YYYY-MM-DD" and a time "HH:MM" in the browser's time zone for an ISO instant.
function localParts(iso: string): { date: string; time: string } {
  const d = new Date(iso)
  return {
    date: `${String(d.getFullYear())}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`,
    time: `${pad(d.getHours())}:${pad(d.getMinutes())}`,
  }
}

// How an entry's date reads: "Wed, 4 Nov 2026" for a day without a time (that very day, whatever the time zone), and the same with the
// time in the browser's zone ("Thu, 5 Nov 2026, 10:30") when there is one. locale is for tests; the browser's own is the default.
export function whenText(m: Pick<ApplicationMessage, 'occurredAt' | 'hasTime'>, locale?: string): string {
  if (!m.hasTime) {
    const [y, mo, d] = m.occurredAt.split('-').map(Number)
    return new Date(Date.UTC(y, mo - 1, d)).toLocaleDateString(locale, {
      weekday: 'short',
      day: 'numeric',
      month: 'short',
      year: 'numeric',
      timeZone: 'UTC',
    })
  }
  return new Date(m.occurredAt).toLocaleString(locale, {
    weekday: 'short',
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

// Has the entry not happened yet (a planned meeting)? A day without a time is planned only until that day is over.
export function isPlanned(m: Pick<ApplicationMessage, 'occurredAt' | 'hasTime'>, now: Date): boolean {
  if (m.hasTime) return new Date(m.occurredAt).getTime() > now.getTime()
  const today = `${String(now.getFullYear())}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`
  return m.occurredAt > today
}

// The newest entry that has happened (what the card highlights), or '' when there is none.
export function latestHappenedId(list: readonly ApplicationMessage[], now: Date): string {
  const happened = sortMessages(list).filter((m) => !isPlanned(m, now))
  return happened.length === 0 ? '' : happened[happened.length - 1].id
}

// Who the other side is: a stored recruiter's name (or the free name) and the address, number or handle: "Rita Free (rita@x.de)".
export function counterpartText(m: Pick<ApplicationMessage, 'counterpartName' | 'counterpartContact'>, recruiterName: string): string {
  const name = recruiterName.trim() !== '' ? recruiterName.trim() : m.counterpartName.trim()
  const contact = m.counterpartContact.trim()
  if (name === '') return contact
  return contact === '' ? name : `${name} (${contact})`
}

// The entry as the form holds it: the date and the optional time are separate, in the browser's time zone.
export interface MessageForm {
  direction: ApplicationDirection
  channel: ApplicationChannel
  channelLabel: string
  date: string
  time: string
  recruiterId: string
  counterpartName: string
  counterpartContact: string
  subject: string
  body: string
  cvDocumentId: string
  externalUrl: string
}

// A new entry: something the person sent, by e-mail, today, with no time yet.
export function emptyForm(now: Date): MessageForm {
  return {
    direction: 'sent',
    channel: 'email',
    channelLabel: '',
    date: `${String(now.getFullYear())}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`,
    time: '',
    recruiterId: '',
    counterpartName: '',
    counterpartContact: '',
    subject: '',
    body: '',
    cvDocumentId: '',
    externalUrl: '',
  }
}

export function formFromMessage(m: ApplicationMessage): MessageForm {
  const when = m.hasTime ? localParts(m.occurredAt) : { date: m.occurredAt, time: '' }
  return {
    direction: m.direction,
    channel: m.channel,
    channelLabel: m.channelLabel,
    date: when.date,
    time: when.time,
    recruiterId: m.recruiterId,
    counterpartName: m.counterpartName,
    counterpartContact: m.counterpartContact,
    subject: m.subject,
    body: m.body,
    cvDocumentId: m.cvDocumentId,
    externalUrl: m.externalUrl,
  }
}

const DATE = /^(\d{4})-(\d{2})-(\d{2})$/
const TIME = /^([01]\d|2[0-3]):([0-5]\d)$/

function realDate(text: string): boolean {
  const m = DATE.exec(text)
  if (!m) return false
  const [y, mo, d] = [Number(m[1]), Number(m[2]), Number(m[3])]
  if (y < 1970 || y > 2100) return false
  const probe = new Date(Date.UTC(y, mo - 1, d))
  return probe.getUTCFullYear() === y && probe.getUTCMonth() === mo - 1 && probe.getUTCDate() === d
}

function longerThan(text: string, max: number): boolean {
  return Array.from(text.trim()).length > max
}

// What stops a form from being sent, in words ('' when it can be): the same checks the server makes, so the form says it first.
export function formProblem(f: MessageForm): string {
  if (!DIRECTIONS.some((x) => x.value === f.direction)) return 'Choose whether it was sent, received or a note'
  if (!CHANNELS.some((x) => x.value === f.channel)) return 'Choose the channel'
  if (!realDate(f.date)) return 'The date must be a real date between 1970 and 2100'
  if (f.time.trim() !== '' && !TIME.test(f.time.trim())) return 'The time must look like 14:30'
  for (const [name, v] of [
    ['The platform or app name', f.channelLabel],
    ['The subject', f.subject],
    ['The name', f.counterpartName],
    ['The address, number or handle', f.counterpartContact],
  ] as const) {
    if (longerThan(v, MAX_SHORT_TEXT)) return `${name} is too long (at most ${String(MAX_SHORT_TEXT)} characters)`
  }
  if (longerThan(f.body, MAX_BODY)) return `The text is too long (at most ${String(MAX_BODY)} characters)`
  const url = f.externalUrl.trim()
  if (url !== '') {
    if (longerThan(url, MAX_URL)) return `The link is too long (at most ${String(MAX_URL)} characters)`
    if (!/^https?:\/\/[^\s/]+/i.test(url)) return 'The link must start with http:// or https://'
  }
  return ''
}

// The form as the server takes it: a date alone when there is no time, else the instant in UTC. The texts are trimmed (the server does it
// too); an empty text clears a field on an update.
export function toInput(f: MessageForm): ApplicationMessageInput {
  const time = f.time.trim()
  let occurredAt = f.date
  if (time !== '') {
    const [y, mo, d] = f.date.split('-').map(Number)
    const [h, mi] = time.split(':').map(Number)
    occurredAt = `${new Date(y, mo - 1, d, h, mi).toISOString().slice(0, 19)}Z`
  }
  return {
    direction: f.direction,
    channel: f.channel,
    channelLabel: f.channelLabel.trim(),
    occurredAt,
    subject: f.subject.trim(),
    body: f.body.trim(),
    recruiterId: f.recruiterId,
    counterpartName: f.counterpartName.trim(),
    counterpartContact: f.counterpartContact.trim(),
    cvDocumentId: f.cvDocumentId,
    externalUrl: f.externalUrl.trim(),
  }
}

// "No messages yet", "1 entry", "3 entries".
export function entryCountText(n: number): string {
  return n === 0 ? 'No entries yet' : n === 1 ? '1 entry' : `${String(n)} entries`
}
