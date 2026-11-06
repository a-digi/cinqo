import { useRef, useState } from 'react'
import { PickError, testPickChoices, type Choices, type PickCandidates, type PickTest, type PickTestJob } from './jobDetailPick'
import {
  addPart,
  addToList,
  chars,
  describePickFailure,
  descriptionHelpFor,
  duplicateBoxes,
  MAX_CUT_AFTER,
  MAX_LIST,
  MAX_PARTS,
  movePart,
  removeFromList,
  removePart,
  ruleProblems,
  shortValue,
  unusedSuggestions,
  updatePart,
  type HeadingMode,
  type PartDraft,
  type RuleDraft,
} from './jobDetailPickRules'

const buttonClass =
  'rounded-md border border-blue-300 bg-white px-2 py-0.5 text-xs font-medium text-blue-800 hover:bg-blue-100 disabled:cursor-not-allowed disabled:opacity-50'
const primaryClass =
  'rounded-md bg-blue-700 px-2.5 py-1 text-xs font-medium text-white hover:bg-blue-800 disabled:cursor-not-allowed disabled:opacity-50'
const inputClass = 'rounded-md border border-blue-200 bg-white px-2 py-1 text-xs'
const SHOWN_TEXT = 2500

function errorText(err: unknown): string {
  if (err instanceof PickError) return describePickFailure(err.message, err.advice)
  return err instanceof Error ? err.message : 'Something went wrong.'
}

// A list of texts (selectors to leave out, texts to stop at): the chips, the suggestions not yet used, and a field to add one.
function ListEditor({
  label,
  items,
  max,
  suggestions,
  placeholder,
  onChange,
}: {
  label: string
  items: string[]
  max: number
  suggestions: { text: string; hint: string; risky?: boolean }[]
  placeholder: string
  onChange: (items: string[]) => void
}) {
  const [draft, setDraft] = useState('')
  const unused = unusedSuggestions(
    suggestions.map((s) => s.text),
    items,
  )
  return (
    <div className="mt-2">
      <div className="font-medium">{label}</div>
      {items.length === 0 && <p className="text-gray-500">Nothing.</p>}
      <ul className="flex flex-wrap gap-1">
        {items.map((it, i) => (
          <li key={it} className="flex items-center gap-1 rounded-full border border-blue-200 bg-white px-2 py-0.5">
            <code className="text-[11px]">{it}</code>
            <button
              type="button"
              aria-label={`Remove ${it}`}
              className="text-gray-500 hover:text-red-700"
              onClick={() => {
                onChange(removeFromList(items, i))
              }}
            >
              ×
            </button>
          </li>
        ))}
      </ul>
      {unused.length > 0 && (
        <div className="mt-1 flex flex-wrap items-center gap-1 text-gray-600">
          <span>Suggested:</span>
          {suggestions
            .filter((s) => unused.includes(s.text))
            .map((s) => (
              <button
                key={s.text}
                type="button"
                title={s.hint}
                className={`rounded-full border px-2 py-0.5 text-[11px] hover:bg-blue-50 ${s.risky ? 'border-amber-400 text-amber-900' : 'border-gray-300 text-gray-800'}`}
                onClick={() => {
                  onChange(addToList(items, s.text, max))
                }}
              >
                + {s.text}
              </button>
            ))}
        </div>
      )}
      <div className="mt-1 flex items-center gap-1">
        <input
          type="text"
          value={draft}
          onChange={(e) => {
            setDraft(e.target.value)
          }}
          placeholder={placeholder}
          aria-label={`Add to: ${label}`}
          className={`${inputClass} w-64`}
        />
        <button
          type="button"
          className={buttonClass}
          disabled={draft.trim() === '' || items.length >= max}
          onClick={() => {
            onChange(addToList(items, draft, max))
            setDraft('')
          }}
        >
          Add
        </button>
      </div>
    </div>
  )
}

function PartCard({
  index,
  count,
  part,
  page,
  onChange,
  onMove,
  onRemove,
}: {
  index: number
  count: number
  part: PartDraft
  page: PickCandidates
  onChange: (patch: Partial<PartDraft>) => void
  onMove: (by: -1 | 1) => void
  onRemove: () => void
}) {
  const help = descriptionHelpFor(page, part.include)
  const boxes = page.ranked.description.slice(0, 8)
  return (
    <div className="rounded-md border border-blue-200 bg-white p-2" data-testid={`rule-part-${index + 1}`}>
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-semibold">Part {index + 1}</span>
        <button
          type="button"
          className={buttonClass}
          disabled={index === 0}
          onClick={() => {
            onMove(-1)
          }}
          aria-label={`Move part ${index + 1} up`}
        >
          ↑
        </button>
        <button
          type="button"
          className={buttonClass}
          disabled={index === count - 1}
          onClick={() => {
            onMove(1)
          }}
          aria-label={`Move part ${index + 1} down`}
        >
          ↓
        </button>
        <button type="button" className={buttonClass} disabled={count <= 1} onClick={onRemove}>
          Remove this part
        </button>
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-2">
        <label className="flex items-center gap-1">
          <span className="font-medium">Box</span>
          <input
            type="text"
            value={part.include}
            onChange={(e) => {
              onChange({ include: e.target.value })
            }}
            placeholder="a CSS selector, for example #job-description"
            className={`${inputClass} w-72 font-mono`}
          />
        </label>
        <label className="flex items-center gap-1">
          <span className="text-gray-600">or choose</span>
          <select
            value=""
            aria-label={`Choose the box of part ${index + 1}`}
            className={inputClass}
            onChange={(e) => {
              if (e.target.value !== '') onChange({ include: e.target.value })
            }}
          >
            <option value="">a box of the page…</option>
            {boxes.map((b) => (
              <option key={b.selector} value={b.selector}>
                {shortValue(b.value, 50)} ({chars(b.chars)})
              </option>
            ))}
          </select>
        </label>
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-2">
        <label className="flex items-center gap-1">
          <span className="font-medium">Heading</span>
          <select
            value={part.heading}
            className={inputClass}
            onChange={(e) => {
              onChange({ heading: e.target.value as HeadingMode })
            }}
          >
            <option value="none">none added</option>
            <option value="auto">the heading just before the box</option>
            <option value="selector">the text of an element</option>
          </select>
        </label>
        {part.heading === 'selector' && (
          <input
            type="text"
            value={part.headingSelector}
            onChange={(e) => {
              onChange({ headingSelector: e.target.value })
            }}
            placeholder="selector of the heading"
            aria-label={`Heading selector of part ${index + 1}`}
            className={`${inputClass} w-56 font-mono`}
          />
        )}
      </div>
      <ListEditor
        label="Leave out of this box"
        items={part.exclude}
        max={MAX_LIST}
        suggestions={(help?.excludes ?? []).map((e) => ({
          text: e.selector,
          hint: `${e.kind}, ${chars(e.chars)}${e.value ? `: ${shortValue(e.value, 60)}` : ''}${e.risky ? ' — check the preview before keeping this one' : ''}`,
          risky: e.risky,
        }))}
        placeholder="a selector, for example .share-buttons"
        onChange={(exclude) => {
          onChange({ exclude })
        }}
      />
      <ListEditor
        label="Stop at the first line that starts with"
        items={part.cutAfter}
        max={MAX_CUT_AFTER}
        suggestions={(help?.cutAfter ?? []).map((t) => ({ text: t, hint: 'the box contains this line' }))}
        placeholder="a text, for example Similar jobs"
        onChange={(cutAfter) => {
          onChange({ cutAfter })
        }}
      />
    </div>
  )
}

function TryResult({ job, test }: { job: PickTestJob; test: PickTest }) {
  const [all, setAll] = useState(false)
  const text = job.values.description
  const report = job.report
  const flags = test.flags.filter((f) => f.field === 'description' || f.field === undefined)
  return (
    <div className="mt-2 space-y-2 rounded-md border border-blue-200 bg-white p-2" data-testid="rule-try-result">
      <div className="font-medium">
        “{job.title}”: {chars(job.descriptionChars)}
        {job.storedChars > 0 ? ` (saved now: ${chars(job.storedChars)})` : ''}
      </div>
      {report?.truncated && (
        <p className="text-amber-900">
          Cut at the limit: {chars(report.chars)} of {chars(report.fullLength)} kept.
        </p>
      )}
      {flags.map((f) => (
        <p
          key={`${f.code}|${f.field ?? ''}`}
          className={f.severity === 'blocking' ? 'text-red-700' : f.severity === 'warning' ? 'text-amber-900' : 'text-gray-600'}
        >
          {f.message}
        </p>
      ))}
      {report?.errors && report.errors.length > 0 && <p className="text-red-700">{report.errors.join(' ')}</p>}
      {report?.parts && report.parts.length > 0 && (
        <table className="w-full text-left">
          <thead>
            <tr className="text-gray-600">
              <th className="pr-2 font-medium">Part</th>
              <th className="pr-2 font-medium">Boxes found</th>
              <th className="pr-2 font-medium">Text</th>
              <th className="pr-2 font-medium">Left out</th>
              <th className="font-medium">Note</th>
            </tr>
          </thead>
          <tbody>
            {report.parts.map((p) => (
              <tr key={p.index}>
                <td className="pr-2">{p.index}</td>
                <td className="pr-2">{p.matches}</td>
                <td className="pr-2">{chars(p.chars)}</td>
                <td className="pr-2">{p.removed > 0 ? chars(p.removed) : '—'}</td>
                <td className="text-amber-900">
                  {p.notFound
                    ? 'no box found on this page'
                    : p.overlap
                      ? 'overlaps another part'
                      : p.cut > 0
                        ? `stopped, ${chars(p.cut)} dropped`
                        : ''}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {report?.outline && report.outline.length > 0 && (
        <div>
          <span className="font-medium">Headings found: </span>
          {report.outline.join(' · ')}
        </div>
      )}
      {report?.removed && report.removed.length > 0 && (
        <div>
          <div className="font-medium">
            What was left out
            {report.removedTotal && report.removedTotal > report.removed.length
              ? ` (the first ${report.removed.length} of ${report.removedTotal})`
              : ''}
          </div>
          <ul className="list-disc pl-5 text-gray-800">
            {report.removed.map((r, i) => (
              <li key={`${r.part}|${r.kind}|${r.selector ?? ''}|${i}`}>
                part {r.part},{' '}
                {r.kind === 'consent'
                  ? 'a cookie or consent box'
                  : r.kind === 'cutAfter'
                    ? `stopped at “${r.selector ?? ''}”`
                    : (r.selector ?? 'an element')}
                : {chars(r.chars)} — <span className="text-gray-600">{shortValue(r.text, 100)}</span>
              </li>
            ))}
          </ul>
          <p className="text-gray-600">Check that nothing real is in this list.</p>
        </div>
      )}
      {job.diff && (
        <div>
          <span className="font-medium">Compared with the description saved now: </span>
          {job.storedChars === 0
            ? 'there is none yet.'
            : job.diff.unchanged
              ? 'the same.'
              : `${job.diff.added} lines added, ${job.diff.removed} removed, ${job.diff.kept} kept.`}
          {job.diff.firstAdded.length > 0 && job.storedChars > 0 && (
            <ul className="list-disc pl-5 text-green-800">
              {job.diff.firstAdded.map((l) => (
                <li key={`a|${l}`}>+ {shortValue(l, 100)}</li>
              ))}
            </ul>
          )}
          {job.diff.firstRemoved.length > 0 && (
            <ul className="list-disc pl-5 text-red-800">
              {job.diff.firstRemoved.map((l) => (
                <li key={`r|${l}`}>− {shortValue(l, 100)}</li>
              ))}
            </ul>
          )}
        </div>
      )}
      <div>
        <div className="font-medium">The text as it would be saved</div>
        <pre className="mt-1 max-h-64 overflow-auto whitespace-pre-wrap rounded-md bg-gray-900 p-2 font-mono text-[11px] text-gray-100">
          {all ? text : text.slice(0, SHOWN_TEXT)}
          {!all && text.length > SHOWN_TEXT ? '\n…' : ''}
        </pre>
        {text.length > SHOWN_TEXT && (
          <button
            type="button"
            className={`${buttonClass} mt-1`}
            onClick={() => {
              setAll((a) => !a)
            }}
          >
            {all ? 'Show less' : 'Show all of it'}
          </button>
        )}
      </div>
    </div>
  )
}

// The editor of a description rule (plan 186, step 7): the parts (the boxes to take, in order), what to leave out of each, a heading, where
// to stop, the format and the limit; and a try-out on a saved job that shows the text, what was left out and what would change.
export function DescriptionRuleEditor({
  linkId,
  page,
  draft,
  choices,
  sampleJobId,
  onChange,
  onUseBox,
}: {
  linkId: string
  page: PickCandidates
  draft: RuleDraft
  choices: Choices
  sampleJobId: string | null
  onChange: (d: RuleDraft) => void
  onUseBox: () => void
}) {
  const [trying, setTrying] = useState(false)
  const [tryError, setTryError] = useState('')
  const [result, setResult] = useState<PickTest | null>(null)
  const seq = useRef(0)
  const problems = ruleProblems(draft)
  const dup = duplicateBoxes(draft)

  function tryIt() {
    if (!sampleJobId) return
    const n = ++seq.current
    setTrying(true)
    setTryError('')
    testPickChoices(linkId, choices, [sampleJobId])
      .then((t) => {
        if (n !== seq.current) return
        setResult(t)
      })
      .catch((err: unknown) => {
        if (n !== seq.current) return
        setResult(null)
        setTryError(errorText(err))
      })
      .finally(() => {
        if (n === seq.current) setTrying(false)
      })
  }

  return (
    <div className="rounded-md border border-blue-300 bg-white p-2" data-testid="rule-editor">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-semibold text-gray-900">Description rule *</span>
        <span className="flex-1 text-gray-600">The boxes to take, what to leave out of them, and how the text is written.</span>
        <button type="button" className={buttonClass} onClick={onUseBox}>
          Use one box instead
        </button>
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-4">
        <fieldset className="flex items-center gap-2">
          <legend className="sr-only">Format</legend>
          <span className="font-medium">Format</span>
          <label className="flex items-center gap-1">
            <input
              type="radio"
              name="rule-format"
              checked={draft.format === 'plain'}
              onChange={() => {
                onChange({ ...draft, format: 'plain' })
              }}
            />
            as the page has it
          </label>
          <label className="flex items-center gap-1">
            <input
              type="radio"
              name="rule-format"
              checked={draft.format === 'structured'}
              onChange={() => {
                onChange({ ...draft, format: 'structured' })
              }}
            />
            headings on their own lines, “- ” before list items
          </label>
        </fieldset>
        <label className="flex items-center gap-1">
          <span className="font-medium">Longest text</span>
          <input
            type="text"
            inputMode="numeric"
            value={draft.limitText}
            onChange={(e) => {
              onChange({ ...draft, limitText: e.target.value })
            }}
            aria-label="Longest description in characters"
            className={`${inputClass} w-24`}
          />
          <span className="text-gray-600">characters</span>
        </label>
      </div>
      <div className="mt-2 space-y-2">
        {draft.parts.map((p, i) => (
          <PartCard
            key={i}
            index={i}
            count={draft.parts.length}
            part={p}
            page={page}
            onChange={(patch) => {
              onChange(updatePart(draft, i, patch))
            }}
            onMove={(by) => {
              onChange(movePart(draft, i, by))
            }}
            onRemove={() => {
              onChange(removePart(draft, i))
            }}
          />
        ))}
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-2">
        <button
          type="button"
          className={buttonClass}
          disabled={draft.parts.length >= MAX_PARTS}
          onClick={() => {
            onChange(addPart(draft))
          }}
        >
          Add a part
        </button>
        <span className="text-gray-600">Several boxes are read in the order of the parts.</span>
      </div>
      {problems.map((p) => (
        <p key={p} className="mt-1 text-red-700">
          {p}
        </p>
      ))}
      {dup.length > 0 && (
        <p className="mt-1 text-amber-900">Two parts take the same box ({dup.join(', ')}): its text would be there twice.</p>
      )}
      <div className="mt-3 flex flex-wrap items-center gap-2">
        <button type="button" className={primaryClass} disabled={trying || problems.length > 0 || !sampleJobId} onClick={tryIt}>
          {trying ? 'Trying it…' : 'Try it on the sample job'}
        </button>
        {!sampleJobId && <span className="text-gray-600">Choose one of the saved jobs as the sample page to try the rule.</span>}
      </div>
      {tryError && (
        <p role="alert" className="mt-1 text-red-700">
          {tryError}
        </p>
      )}
      {result &&
        result.jobs.length > 0 &&
        (result.jobs[0].error ? (
          <p className="mt-1 text-red-700">{result.jobs[0].error}</p>
        ) : (
          <TryResult job={result.jobs[0]} test={result} />
        ))}
    </div>
  )
}
