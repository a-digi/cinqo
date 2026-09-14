import { useEffect, useState, type FormEvent } from 'react'
import { fetchJobs, removeJob, type Job } from './api'

// No "add job" affordance — jobs are populated by the AI's own
// crawling workflow (step 4), never hand-entered here. See this
// step's own open question 2.
export function JobsPage() {
  const [query, setQuery] = useState('')
  const [location, setLocation] = useState('')
  const [jobs, setJobs] = useState<Job[]>([])
  const [total, setTotal] = useState(0)
  const [error, setError] = useState('')

  function load() {
    setError('')
    fetchJobs(query, location)
      .then((result) => {
        setJobs(result.jobs)
        setTotal(result.total)
      })
      .catch((err: Error) => setError(err.message))
  }

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  function handleSearch(e: FormEvent) {
    e.preventDefault()
    load()
  }

  function handleRemove(id: string) {
    setError('')
    removeJob(id)
      .then(load)
      .catch((err: Error) => setError(err.message))
  }

  return (
    <div className="max-w-4xl p-6 font-sans text-gray-900">
      <h1 className="mb-1.5 text-xl font-semibold">Saved Jobs</h1>
      <p className="mb-5 text-sm text-gray-500">
        Job postings the AI has crawled and saved on your behalf, showing {jobs.length} of {total}.
      </p>

      <form onSubmit={handleSearch} className="mb-4 flex flex-wrap gap-2">
        <input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search title, company, description…"
          className="flex-1 min-w-[180px] rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
        />
        <input
          value={location}
          onChange={(e) => setLocation(e.target.value)}
          placeholder="Location…"
          className="min-w-[140px] rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
        />
        <button
          type="submit"
          className="rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800"
        >
          Search
        </button>
      </form>

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      <div className="overflow-hidden rounded-md border border-gray-200 shadow-sm">
        <table className="w-full text-sm">
          <thead>
            <tr>
              <th className="border-b border-gray-200 bg-gray-50 p-3 text-left font-medium text-gray-500">Title</th>
              <th className="border-b border-gray-200 bg-gray-50 p-3 text-left font-medium text-gray-500">Company</th>
              <th className="border-b border-gray-200 bg-gray-50 p-3 text-left font-medium text-gray-500">Location</th>
              <th className="border-b border-gray-200 bg-gray-50 p-3 text-left font-medium text-gray-500">Posted</th>
              <th className="border-b border-gray-200 bg-gray-50 p-3"></th>
            </tr>
          </thead>
          <tbody>
            {jobs.map((job) => (
              <tr key={job.id} className="last:[&>td]:border-b-0 hover:bg-gray-50">
                <td className="border-b border-gray-200 p-3">
                  <a
                    href={job.sourceUrl}
                    target="_blank"
                    rel="noreferrer"
                    className="text-gray-900 underline decoration-gray-300 hover:decoration-gray-600"
                  >
                    {job.title}
                  </a>
                </td>
                <td className="border-b border-gray-200 p-3">{job.company}</td>
                <td className="border-b border-gray-200 p-3">{job.location}</td>
                <td className="border-b border-gray-200 p-3">{job.postedAt}</td>
                <td className="border-b border-gray-200 p-3">
                  <button
                    type="button"
                    onClick={() => handleRemove(job.id)}
                    className="rounded-md border border-gray-200 px-3 py-1 text-red-700 transition-colors hover:bg-red-50"
                  >
                    Remove
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
