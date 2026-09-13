import { useNavigate } from 'react-router-dom'

// Rendered by AuthGuard (frontend/auth/04) as the default fallback when a
// scope check fails.
export function AccessDeniedPage() {
  const navigate = useNavigate()

  return (
    <div className="flex min-h-64 flex-col items-center justify-center px-4 text-center">
      <div className="mb-4 flex h-14 w-14 items-center justify-center rounded-full bg-red-100">
        <svg className="h-7 w-7 text-red-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.8}>
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126ZM12 15.75h.007v.008H12v-.008Z"
          />
        </svg>
      </div>
      <h1 className="mb-1 text-xl font-semibold text-gray-900">Access Denied</h1>
      <p className="mb-6 max-w-sm text-sm text-gray-500">You don't have permission to view this page.</p>
      <button
        type="button"
        onClick={() => navigate(-1)}
        className="rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
      >
        Go Back
      </button>
    </div>
  )
}
