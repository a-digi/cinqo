// One read of the platforms at a time (plan 243, step 1). refresh() starts a read; a call made while a read is running does not start another: it makes
// one more read run right after it (so a change made while the first read was in flight is not missed), and every caller waits for the same promise.
// What a read gives, and a failure of one, are handed to the two callbacks; a failure never stops the next read.
export interface RefresherDeps<T> {
  read: () => Promise<T>
  onResult: (result: T) => void
  onError: (err: unknown) => void
}

export function createRefresher<T>(deps: RefresherDeps<T>): () => Promise<void> {
  let running: Promise<void> | null = null
  let again = false
  // read through a function: a call made during an await sets it, which the compiler cannot see from the assignment above it
  const askedAgain = () => again

  async function once(): Promise<void> {
    try {
      deps.onResult(await deps.read())
    } catch (err) {
      deps.onError(err)
    }
  }

  return function refresh(): Promise<void> {
    if (running !== null) {
      again = true
      return running
    }
    running = (async () => {
      do {
        again = false
        await once()
      } while (askedAgain())
    })().finally(() => {
      running = null
    })
    return running
  }
}
