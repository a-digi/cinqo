export interface LoadingSpinnerProps {
  label?: string
  className?: string
}

// No API calls, no context reads — controlled purely by props.
export function LoadingSpinner({ label, className = '' }: LoadingSpinnerProps) {
  return (
    <div className={`flex flex-col items-center justify-center gap-3 ${className}`}>
      <div
        role="status"
        aria-label={label ?? 'Loading'}
        className="h-8 w-8 animate-spin rounded-full border-4 border-slate-300 border-t-transparent"
      />
      {label && <span className="text-sm text-slate-600">{label}</span>}
    </div>
  )
}
