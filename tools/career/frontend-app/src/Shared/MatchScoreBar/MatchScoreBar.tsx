// MatchScoreBar — a shared, reusable progress bar showing a 0-100
// "Job Match" score (job_match.go), colored by tier. Controlled
// (score-in only, no fetch calls), so any future feature needing the
// same "colored progress bar for a percentage" shape can reuse it
// unchanged. Boundaries (score, not "greater than X"): 91-100 dark
// green + a bouncing label (reusing the exact robot-bob keyframe
// CrawlPanel's own "Check Progress - AI" indicator already uses, for
// visual consistency across this tool), 81-90 light green, 60-80
// orange, 40-59 violet, 0-39 red. See
// plan/ai/tools/career/step-XX-job-match.md.
export function MatchScoreBar({ score }: { score: number }) {
  const clamped = Math.max(0, Math.min(100, score))
  const isExceptional = clamped >= 91
  const barColor = isExceptional
    ? 'bg-green-700'
    : clamped >= 81
      ? 'bg-green-400'
      : clamped >= 60
        ? 'bg-orange-400'
        : clamped >= 40
          ? 'bg-violet-400'
          : 'bg-red-500'

  return (
    <div className="flex items-center gap-2" title={`Match score: ${clamped}%`}>
      <div className="h-2 w-20 overflow-hidden rounded-full bg-gray-100">
        <div className={`h-full rounded-full ${barColor}`} style={{ width: `${clamped}%` }} />
      </div>
      <span className={`text-xs font-medium text-gray-700 ${isExceptional ? '[animation:robot-bob_1.6s_ease-in-out_infinite]' : ''}`}>
        {clamped}%
      </span>
    </div>
  )
}
