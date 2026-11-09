import { fetchReviewSummaries, type ReviewBrief, type ReviewSummaries } from '../../api/reviews'

export interface BriefAnswer {
  scale: number
  halfStars: boolean
  brief: ReviewBrief
}

type Fetcher = (owner: string, entityType: string, ids: string[]) => Promise<ReviewSummaries>

interface Waiting {
  resolve: (a: BriefAnswer) => void
  reject: (e: unknown) => void
}

// How long a request waits for the others of the same list to arrive before one call is sent for all of them.
export const BATCH_WAIT_MS = 20
// The server answers up to 100 ids per call.
export const BATCH_SIZE = 100

// Collects the "stars of this entity" requests a list makes (one per row, all in the same moment) and sends them as one call per owner and
// entity type, so a table of 50 companies costs one request. The same id asked twice in a moment is sent once and answers both.
export function createSummaryBatcher(fetcher: Fetcher = fetchReviewSummaries, wait: number = BATCH_WAIT_MS) {
  const queues = new Map<string, Map<string, Waiting[]>>()
  const timers = new Map<string, ReturnType<typeof setTimeout>>()

  async function send(owner: string, entityType: string, batch: Map<string, Waiting[]>) {
    const ids = [...batch.keys()]
    for (let i = 0; i < ids.length; i += BATCH_SIZE) {
      const chunk = ids.slice(i, i + BATCH_SIZE)
      try {
        const answer = await fetcher(owner, entityType, chunk)
        for (const id of chunk) {
          const brief = answer.summaries[id] ?? { count: 0, average: 0 }
          for (const w of batch.get(id) ?? []) w.resolve({ scale: answer.scale, halfStars: answer.halfStars, brief })
        }
      } catch (err) {
        for (const id of chunk) for (const w of batch.get(id) ?? []) w.reject(err)
      }
    }
  }

  function request(owner: string, entityType: string, id: string): Promise<BriefAnswer> {
    const key = `${owner}/${entityType}`
    return new Promise<BriefAnswer>((resolve, reject) => {
      let batch = queues.get(key)
      if (!batch) {
        batch = new Map()
        queues.set(key, batch)
      }
      const waiting = batch.get(id) ?? []
      waiting.push({ resolve, reject })
      batch.set(id, waiting)
      if (!timers.has(key)) {
        timers.set(
          key,
          setTimeout(() => {
            timers.delete(key)
            const ready = queues.get(key)
            queues.delete(key)
            if (ready) void send(owner, entityType, ready)
          }, wait),
        )
      }
    })
  }

  return { request }
}

// The one the screen uses.
export const summaryBatcher = createSummaryBatcher()
