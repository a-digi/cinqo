import { useState } from 'react'
import { Modal } from '../Modal/Modal'
import type { MatchedSkill } from '../../api'

// MatchScoreBar — a shared, reusable progress bar showing a 0-100
// "Job Match" score (job_match.go), colored by tier, with the
// specific skills that explain it available on hover (a native
// tooltip) and in a small modal on click. Controlled for its own
// score/skills/title (no fetch calls) but owns its own simple
// open/closed modal state internally — same "a shared component may
// manage its own small interactive UI state" precedent Dropdown/
// Accordion already establish in this tool — so every call site stays
// a one-line `<MatchScoreBar .../>` with no external modal wiring.
// Boundaries (score, not "greater than X"): 91-100 dark green + a
// bouncing label (reusing the exact robot-bob keyframe CrawlPanel's
// own "Check Progress - AI" indicator already uses, for visual
// consistency across this tool), 81-90 light green, 60-80 orange,
// 40-59 violet, 0-39 red. See
// plan/ai/tools/career/step-XX-job-match.md and
// plan/ai/tools/career/step-XX-job-match-skills.md.
// Only literal (exact-wording) matches are shown here — a semantic
// match (matched by meaning, not exact text) is a fuzzier, lower-
// confidence signal that's still allowed to contribute to the score
// itself (job_match_algorithm.go), but is deliberately excluded from
// this display: matched skills shown to the user should be the ones
// that unambiguously and literally appear in the job's own text, not
// an approximate, sometimes-hard-to-justify semantic guess.
export function MatchScoreBar({ score, skills, jobTitle }: { score: number; skills: MatchedSkill[]; jobTitle: string }) {
  const [modalOpen, setModalOpen] = useState(false)
  const literalSkills = skills.filter((s) => s.kind === 'literal')
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
  const tooltip =
    literalSkills.length > 0
      ? `Match score: ${clamped}%\nMatched skills: ${literalSkills.map((s) => s.skill).join(', ')}`
      : `Match score: ${clamped}%\nNo specific skills matched`

  return (
    <>
      <button
        type="button"
        onClick={() => {
          setModalOpen(true)
        }}
        title={tooltip}
        className="flex items-center gap-2"
      >
        <div className="h-2 w-20 overflow-hidden rounded-full bg-gray-100">
          <div className={`h-full rounded-full ${barColor}`} style={{ width: `${clamped}%` }} />
        </div>
        <span className={`text-xs font-medium text-gray-700 ${isExceptional ? '[animation:robot-bob_1.6s_ease-in-out_infinite]' : ''}`}>
          {clamped}%
        </span>
      </button>
      <Modal
        open={modalOpen}
        title={jobTitle}
        onClose={() => {
          setModalOpen(false)
        }}
      >
        <p className="mb-3 text-sm text-gray-500">Match score: {clamped}%</p>
        {literalSkills.length > 0 ? (
          <div className="flex flex-wrap gap-1.5">
            {literalSkills.map((skill) => (
              <span key={skill.skill} className="rounded-full border border-gray-200 bg-gray-100 px-2.5 py-1 text-xs text-gray-700">
                {skill.skill}
              </span>
            ))}
          </div>
        ) : (
          <p className="text-sm text-gray-400">No specific skills matched.</p>
        )}
      </Modal>
    </>
  )
}
