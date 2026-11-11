import { useEffect, useMemo, useRef, useState } from 'react'
import type { PortalLink } from '../../api'
import { DescriptionRuleEditor } from './DescriptionRuleEditor'
import { VerifyPanel } from './VerifyPanel'
import { WindowCheckPrompt } from '../WindowCheckPrompt'
import { DetailPageView } from './DetailPageView'
import { FieldStrip } from './FieldStrip'
import { TempFilesLine } from '../PageFrame/TempFilesLine'
import { deleteSnapshot, WINDOW_CHECK_CODE } from '../linkCrawl'
import { numberOfSelector, pickFromView, viewByNumber } from './fieldMarks'
import {
  loadPickCandidates,
  PickError,
  previewPickChoices,
  type PickCandidates,
  type PickField,
  type PickPreview,
  type Ranked,
  type ViewElement,
} from './jobDetailPick'
import {
  canPreview,
  DATA_FIELDS,
  draftFromBox,
  describePickFailure,
  draftFromStatedDescription,
  FIELD_NAMES,
  fillNote,
  filterElements,
  furnitureNote,
  hintsFor,
  isStructuredPick,
  lossyNote,
  originLabel,
  pickFromElement,
  pickFromHint,
  pickFromRanked,
  samePick,
  shortValue,
  shownSuggestions,
  startingPicks,
  startingReplaceTitle,
  startingRule,
  statedDescription,
  toChoices,
  whyText,
  type Pick,
  type Picks,
  type RuleDraft,
  NO_PICKS,
} from './jobDetailPickRules'
import { InstructionSection } from '../InstructionSection'
import { buttonClass, inputClass, primaryClass } from '../instructionStyles'

function errorText(err: unknown): string {
  if (err instanceof PickError) return describePickFailure(err.message, err.advice)
  return err instanceof Error ? err.message : 'Something went wrong.'
}

// One field of the job: where on the sample page it is read from. The ranked suggestions, then any text element of the page.
function FieldRow({
  field,
  page,
  pick,
  required,
  onPick,
}: {
  field: PickField
  page: PickCandidates
  pick: Pick | null
  required: boolean
  onPick: (p: Pick | null) => void
}) {
  const [expanded, setExpanded] = useState(false)
  const [searching, setSearching] = useState(false)
  const [query, setQuery] = useState('')
  const suggestions: Ranked[] = useMemo(() => shownSuggestions(page, field), [page, field])
  const hints = useMemo(() => hintsFor(page, field), [page, field])
  const elements = useMemo(() => filterElements(page.elements, query), [page.elements, query])
  const open = expanded || pick === null
  const name = FIELD_NAMES[field]
  const readsAttribute = pick?.attribute && pick.attribute !== 'text' ? pick.attribute : ''

  return (
    <div className="rounded-xl border border-gray-200 bg-white p-3" data-testid={`job-detail-pick-row-${field}`}>
      <div className="flex flex-wrap items-center gap-2">
        <span className="w-24 shrink-0 font-semibold text-gray-900">
          {name}
          {required && <span className="ml-0.5 text-red-700"> *</span>}
        </span>
        {pick ? (
          <span className="min-w-0 flex-1 text-gray-800">
            {pick.value ? (
              <span className="font-medium">{shortValue(pick.value)}</span>
            ) : (
              <span className="text-gray-500">(kept from the stored instructions)</span>
            )}{' '}
            {isStructuredPick(pick) ? (
              <span className="rounded border border-green-200 bg-green-50 px-1 text-[11px] text-green-700">
                from the page&apos;s own data ({pick.source}: {pick.path})
              </span>
            ) : (
              <code className="rounded bg-gray-100 px-1 text-[11px]">{pick.selector}</code>
            )}
            {readsAttribute && (
              <span className="ml-1 rounded bg-gray-100 px-1 text-[11px] text-gray-700">reads the {readsAttribute} attribute</span>
            )}
          </span>
        ) : (
          <span className="flex-1 text-gray-500">{required ? 'Choose where the description is.' : 'Not on this page (left out).'}</span>
        )}
        <button
          type="button"
          className={buttonClass}
          onClick={() => {
            setExpanded((e) => !e)
          }}
        >
          {open && pick ? 'Close' : pick ? 'Change' : 'Show the suggestions'}
        </button>
        {pick && !required && (
          <button
            type="button"
            className={buttonClass}
            onClick={() => {
              onPick(null)
              setExpanded(false)
            }}
          >
            Not on this page
          </button>
        )}
      </div>
      {open && (
        <div className="mt-2 space-y-1">
          {hints.map((h) => {
            const hint = pickFromHint(h)
            const chosen = samePick(pick, hint)
            return (
              <label
                key={`${h.source}|${h.path}`}
                aria-label={`The page states: ${shortValue(h.value, 140)}`}
                className={`flex cursor-pointer items-start gap-2 rounded-md border px-2 py-1 ${chosen ? 'border-gray-900 bg-gray-50' : 'border-gray-200 bg-gray-50 hover:bg-gray-50'}`}
                data-testid="job-detail-pick-stated"
              >
                <input
                  type="radio"
                  name={`pick-${field}`}
                  checked={chosen}
                  onChange={() => {
                    onPick(hint)
                  }}
                  className="mt-0.5"
                />
                <span className="min-w-0 flex-1">
                  <span className="font-medium">{shortValue(h.value, 140)}</span>
                  <span className="ml-1 text-green-700">from the page&apos;s own data · {h.reason}</span>
                </span>
              </label>
            )
          })}
          {suggestions.length === 0 && hints.length === 0 && (
            <p className="text-gray-600">The page offers no suggestion for this field. Pick one from the page&apos;s text below.</p>
          )}
          {suggestions.map((s) => {
            const chosen = samePick(pick, pickFromRanked(s))
            return (
              <label
                key={`${s.selector}|${s.attribute}`}
                title={whyText(s)}
                aria-label={shortValue(s.value, 140)}
                className={`flex cursor-pointer items-start gap-2 rounded-md border px-2 py-1 ${chosen ? 'border-gray-900 bg-gray-50' : 'border-gray-200 hover:bg-gray-50'}`}
              >
                <input
                  type="radio"
                  name={`pick-${field}`}
                  checked={chosen}
                  onChange={() => {
                    onPick(pickFromRanked(s))
                  }}
                  className="mt-0.5"
                />
                <span className="min-w-0 flex-1">
                  <span className="font-medium">{shortValue(s.value, 140)}</span>
                  <span className="ml-1 text-gray-500">
                    {s.attribute && s.attribute !== 'text' ? `[${s.attribute}] ` : ''}
                    {field === 'description' ? `${s.chars} characters` : s.tag}
                    {s.score >= 5 ? ' · likely' : ''}
                  </span>
                  <span className="block truncate font-mono text-[11px] text-gray-500">{s.selector}</span>
                </span>
              </label>
            )
          })}
          <div>
            <button
              type="button"
              className={buttonClass}
              onClick={() => {
                setSearching((v) => !v)
              }}
            >
              {searching ? 'Hide the page’s text' : 'Pick from the page’s text'}
            </button>
          </div>
          {searching && (
            <div className="space-y-1">
              <input
                type="search"
                value={query}
                onChange={(e) => {
                  setQuery(e.target.value)
                }}
                placeholder="Filter by text or selector"
                aria-label={`Filter the page's text for ${name}`}
                className={`w-64 ${inputClass}`}
              />
              <ul className="max-h-48 overflow-y-auto rounded-md border border-gray-200 bg-white">
                {elements.length === 0 && <li className="px-2 py-1 text-gray-600">Nothing on the page matches.</li>}
                {elements.map((e) => (
                  <li key={e.selector}>
                    <button
                      type="button"
                      onClick={() => {
                        onPick(pickFromElement(e))
                        setExpanded(false)
                      }}
                      className="flex w-full items-baseline gap-2 px-2 py-1 text-left hover:bg-gray-50"
                    >
                      <span className="min-w-0 flex-1 truncate">{shortValue(e.text, 120)}</span>
                      <span className="shrink-0 font-mono text-[11px] text-gray-500">{e.tag}</span>
                    </button>
                  </li>
                ))}
              </ul>
              {page.truncated && <p className="text-gray-600">The page has more text elements than are listed here.</p>}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

// "Pick the data of a job page" (plan 186, step 6): choose a sample job page, see where the page says the title, the company, the
// location, the posted date and the description, choose each, and see the instructions those choices make. No AI is involved. This
// part only previews: saving and verifying on sample jobs come in the next parts, so nothing is stored here.
export function JobDetailPickPanel({ link, onSaved }: { link: PortalLink; onSaved: (yaml: string) => void }) {
  const [open, setOpen] = useState(false)
  const [page, setPage] = useState<PickCandidates | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [sample, setSample] = useState<string>('default') // 'default', a saved job's id, or 'url'
  const [customUrl, setCustomUrl] = useState('')
  const [picks, setPicks] = useState<Picks>(NO_PICKS)
  // The description rule being edited; null while the description is read from one box.
  const [rule, setRule] = useState<RuleDraft | null>(null)
  // Let the title the page states replace the job's saved title when its page is crawled (off by default).
  const [replaceTitle, setReplaceTitle] = useState(false)
  const [preview, setPreview] = useState<PickPreview | null>(null)
  const [previewError, setPreviewError] = useState('')
  // plan 198: a browser window opened for a check; the person is asked whether it was cleared
  const [windowCheck, setWindowCheck] = useState(false)
  // plan 249: with a page to show, only the page is shown (large) over a strip of the five fields; the rows are one click away ("Show the fields").
  const [fieldsShown, setFieldsShown] = useState(false)
  // Scroll the page to the element of a pick made in the rows; seq makes asking twice for the same one work.
  const [reveal, setReveal] = useState<{ numbers: number[]; seq: number } | null>(null)
  const loadSeq = useRef(0)
  const previewSeq = useRef(0)
  const revealSeq = useRef(0)

  function load(choice: string, url: string) {
    const seq = ++loadSeq.current
    setLoading(true)
    setError('')
    setWindowCheck(false)
    const target = choice === 'url' ? { url: url.trim() } : choice === 'default' ? {} : { jobId: choice }
    loadPickCandidates(link.id, target, undefined, { snapshot: true })
      .then((p) => {
        if (seq !== loadSeq.current) return
        setPage(p)
        setPicks(startingPicks(p))
        setRule(startingRule(p))
        setReplaceTitle(startingReplaceTitle(p))
        if (choice === 'default' && p.samples.length > 0) setSample(p.samples[0].jobId)
      })
      .catch((err: unknown) => {
        if (seq !== loadSeq.current) return
        if (err instanceof PickError && err.code === WINDOW_CHECK_CODE) {
          setWindowCheck(true) // the question replaces the error text
          return
        }
        setError(errorText(err))
      })
      .finally(() => {
        if (seq === loadSeq.current) setLoading(false)
      })
  }

  // First opening: load the default sample (the oldest saved job of the link).
  useEffect(() => {
    if (open && page === null && !loading && error === '') load('default', '')
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  // The picks changed: ask what instructions they make (debounced; a late answer for older picks is dropped).
  useEffect(() => {
    if (!page || !canPreview(picks, rule)) {
      setPreview(null)
      setPreviewError('')
      return
    }
    const seq = ++previewSeq.current
    const timer = setTimeout(() => {
      previewPickChoices(link.id, toChoices(picks, rule, replaceTitle))
        .then((p) => {
          if (seq !== previewSeq.current) return
          setPreview(p)
          setPreviewError('')
        })
        .catch((err: unknown) => {
          if (seq !== previewSeq.current) return
          setPreview(null)
          setPreviewError(errorText(err))
        })
    }, 250)
    return () => {
      clearTimeout(timer)
    }
  }, [picks, rule, replaceTitle, page, link.id])

  // plan 249: a page copy lives as long as the page it belongs to; a new load replaces it and closing or saving the picker removes it.
  const snapshotId = page?.snapshot?.id ?? ''
  useEffect(() => {
    if (snapshotId === '') return
    return () => {
      void deleteSnapshot(snapshotId).catch(() => undefined)
    }
  }, [snapshotId])
  const view: ViewElement[] = useMemo(() => page?.viewElements ?? [], [page])
  const hasView = snapshotId !== '' && view.length > 0

  function setPick(field: PickField, p: Pick | null) {
    setPicks((prev) => ({ ...prev, [field]: p }))
    // a pick made in the rows or the strip is shown on the page
    const n = p && (p.source ?? '') === '' ? numberOfSelector(view, p.selector) : undefined
    if (n !== undefined && hasView) {
      revealSeq.current += 1
      setReveal({ numbers: [n], seq: revealSeq.current })
    }
  }

  // The person answered in the page for the element n: it becomes the pick of the field (the page is what they see, so the pick is what they pointed at), or what it is now is cleared.
  function answerFromPage(n: number, field: PickField | null, now: PickField[], attribute?: string) {
    if (field === null) {
      setPicks((prev) => {
        const next = { ...prev }
        for (const f of now) if (!(f === 'description' && rule !== null)) next[f] = null
        return next
      })
      return
    }
    const v = viewByNumber(view).get(n)
    if (!v) return
    setPicks((prev) => ({ ...prev, [field]: pickFromView(v, attribute) }))
  }

  // The instructions are saved: the modal stays open, but the picker folds away so only the saved state and its button are left, and everything of this round is dropped;
  // its page copy goes with the page (the effect on the copy's id deletes the file).
  function saved(yaml: string) {
    setOpen(false)
    setPage(null)
    setPicks(NO_PICKS)
    setRule(null)
    setPreview(null)
    setPreviewError('')
    setFieldsShown(false)
    setReveal(null)
    onSaved(yaml)
  }

  const note = page ? lossyNote(page.current.lossy) : ''

  // The description row (or its rule editor): one block, shown in step 3, or beside the page under "Show the fields".
  function renderDescription(pg: PickCandidates) {
    return (
      <div className="space-y-2">
        {rule === null ? (
          <>
            <FieldRow
              key={`${pg.url}|description`}
              field="description"
              page={pg}
              pick={picks.description}
              required
              onPick={(p) => {
                setPick('description', p)
              }}
            />
            {picks.description !== null && !isStructuredPick(picks.description) && furnitureNote(pg, picks.description.selector) !== '' && (
              <p className="pl-1 text-amber-900" data-testid="job-detail-pick-furniture">
                {furnitureNote(pg, picks.description.selector)}
              </p>
            )}
            <div className="flex flex-wrap items-center gap-2 pl-1">
              <button
                type="button"
                className={buttonClass}
                onClick={() => {
                  setRule(draftFromBox(picks.description?.selector ?? ''))
                }}
              >
                Read the description with a rule
              </button>
              <span className="text-gray-600">Several boxes, things to leave out, headings, a longer limit than 5,000 characters.</span>
              {statedDescription(pg) && (
                <button
                  type="button"
                  className={buttonClass}
                  data-testid="job-detail-pick-use-stated-description"
                  onClick={() => {
                    setRule(draftFromStatedDescription())
                  }}
                >
                  Use the description the pg states
                </button>
              )}
            </div>
          </>
        ) : (
          <DescriptionRuleEditor
            linkId={link.id}
            page={pg}
            draft={rule}
            choices={toChoices(picks, rule, replaceTitle)}
            sampleJobId={sample !== 'default' && sample !== 'url' ? sample : null}
            onChange={setRule}
            onUseBox={() => {
              const first = rule.parts[0].include.trim()
              if (first !== '' && picks.description === null)
                setPick('description', { selector: first, attribute: '', value: '', from: 'stored' })
              setRule(null)
            }}
          />
        )}
      </div>
    )
  }

  return (
    <div className="text-sm text-gray-800" data-testid="job-detail-pick">
      <div className="min-w-0">
        <h3 className="text-base font-semibold text-gray-900">Pick the data of a job page</h3>
        <p className="mt-0.5 text-sm text-gray-500">
          Choose where a job page says its title, company, location, date and description, without AI.
        </p>
        <div className="mt-3">
          <button
            type="button"
            className={primaryClass}
            onClick={() => {
              setOpen((o) => !o)
            }}
          >
            {open ? 'Close' : 'Choose the data'}
          </button>
        </div>
        {link.jobDetailCrawlInstructions && originLabel(link.jobDetailInstructionsOrigin) !== '' && (
          <span
            className="mt-2 inline-block rounded-full border border-gray-200 bg-white px-2 py-0.5 text-[11px] text-gray-600"
            data-testid="job-detail-origin"
          >
            Instructions now: {originLabel(link.jobDetailInstructionsOrigin)}
          </span>
        )}
      </div>

      {link.jobDetailCrawlInstructions && fillNote(link.jobDetailInstructionsOrigin) !== '' && (
        <p className="mt-3 text-gray-700">{fillNote(link.jobDetailInstructionsOrigin)}</p>
      )}

      {open && (
        <div className="mt-8">
          <InstructionSection
            number={1}
            title="Sample job page"
            description="Choose a saved job of this link, or another address on its own site, and load it."
          >
            <fieldset className="space-y-1" disabled={loading}>
              <legend className="sr-only">Sample job page</legend>
              {page && page.samples.length > 0 && (
                <div className="space-y-1">
                  {page.samples.map((s) => (
                    <label key={s.jobId} className="flex items-center gap-2">
                      <input
                        type="radio"
                        name="pick-sample"
                        checked={sample === s.jobId}
                        onChange={() => {
                          setSample(s.jobId)
                        }}
                      />
                      <span className="min-w-0 flex-1 truncate">{s.title}</span>
                      <span className="shrink-0 text-gray-500">
                        {s.descriptionChars > 0 ? `${s.descriptionChars} characters saved` : 'no description yet'}
                      </span>
                    </label>
                  ))}
                </div>
              )}
              <label className="mt-1 flex items-center gap-2">
                <input
                  type="radio"
                  name="pick-sample"
                  checked={sample === 'url'}
                  onChange={() => {
                    setSample('url')
                  }}
                />
                <span>Another address of this site:</span>
                <input
                  type="url"
                  value={customUrl}
                  onChange={(e) => {
                    setCustomUrl(e.target.value)
                    setSample('url')
                  }}
                  placeholder="https://…"
                  aria-label="Address of a job page on this site"
                  className={`min-w-0 flex-1 ${inputClass}`}
                />
              </label>
              <div className="mt-2 flex items-center gap-2">
                <button
                  type="button"
                  className={primaryClass}
                  disabled={loading || (sample === 'url' && customUrl.trim() === '')}
                  onClick={() => {
                    load(sample, customUrl)
                  }}
                >
                  {loading ? 'Loading the page…' : page ? 'Load this page' : 'Load the page'}
                </button>
                {page && (
                  <span className="text-gray-700">
                    “{page.title || page.finalUrl}” — {page.elements.length} text elements
                    {page.truncated ? ' (more than are listed)' : ''}
                  </span>
                )}
              </div>
            </fieldset>

            {windowCheck && (
              <WindowCheckPrompt
                portalLinkId={link.id}
                onCleared={() => {
                  load('default', '')
                }}
                onCancel={() => {
                  setWindowCheck(false)
                }}
              />
            )}
            {error && (
              <p role="alert" className="text-red-700">
                {error}
              </p>
            )}
          </InstructionSection>

          {page && (
            <>
              <InstructionSection
                number={2}
                title="Fields"
                description="Where the page says the title, the company, the location and the posted date."
              >
                {note && <p className="rounded-md border border-amber-300 bg-amber-50 p-2 text-amber-900">{note}</p>}
                {page.notices.map((n) => (
                  <p
                    key={n.code}
                    role="status"
                    data-testid="page-notice"
                    className="rounded-md border border-amber-300 bg-amber-50 p-2 text-amber-900"
                  >
                    {n.message}
                  </p>
                ))}
                {page.current.yaml !== '' && page.current.choices && (
                  <p className="text-gray-700">This link already has job-detail instructions; the picks below start from them.</p>
                )}
                {hasView && (
                  <>
                    <p className="text-gray-700" data-testid="job-detail-page-hint">
                      Click or select the text of a field in the page and say what it is: the <strong>title</strong>, the company, the
                      location, the posted date or the <strong>description</strong>. Only the description is required.
                    </p>
                    <FieldStrip page={page} view={view} picks={picks} rule={rule} onPick={setPick} />
                    <div className="flex flex-wrap items-center gap-3">
                      <button
                        type="button"
                        className={buttonClass}
                        aria-expanded={fieldsShown}
                        onClick={() => {
                          setFieldsShown((v) => !v)
                        }}
                      >
                        {fieldsShown ? 'Hide the fields' : 'Show the fields'}
                      </button>
                      <TempFilesLine open={open} snapshotId={snapshotId} />
                    </div>
                  </>
                )}
                <div
                  className={hasView && fieldsShown ? 'grid gap-4 lg:grid-cols-2 lg:items-start' : ''}
                  data-testid={hasView ? 'job-detail-fields-and-view' : undefined}
                >
                  {(!hasView || fieldsShown) && (
                    <div className="min-w-0 space-y-3">
                      <p className="text-gray-600">
                        Only the description is required. Leave any other field as &quot;Not on this page&quot;: a field the page does not
                        show never stops you from saving.
                      </p>
                      <div className="space-y-2">
                        {DATA_FIELDS.map((f) => (
                          <div key={`${page.url}|${f}`} className="space-y-1">
                            <FieldRow
                              field={f}
                              page={page}
                              pick={picks[f]}
                              required={false}
                              onPick={(p) => {
                                setPick(f, p)
                              }}
                            />
                            {f === 'title' && picks.title !== null && (
                              <label className="ml-1 flex items-start gap-2 text-gray-800" data-testid="job-detail-pick-replace-title">
                                <input
                                  type="checkbox"
                                  checked={replaceTitle}
                                  onChange={(e) => {
                                    setReplaceTitle(e.target.checked)
                                  }}
                                  className="mt-0.5"
                                />
                                <span>
                                  Replace the job&apos;s title with this one when its page is crawled. Off: the title is only checked
                                  against the saved one.
                                  <span className="block text-gray-600">
                                    Use it when the listing&apos;s titles are poor. A later crawl of the listing may add the job again under
                                    its old title. Each change is written in the run log; jobs you added by hand keep their title.
                                  </span>
                                </span>
                              </label>
                            )}
                          </div>
                        ))}
                      </div>

                      {page.structuredHints.length > 0 && (
                        <div className="rounded-xl border border-gray-200 bg-white p-3" data-testid="job-detail-pick-hints">
                          <div className="font-medium">What the page itself says about the job</div>
                          <ul className="mt-1 list-disc pl-5 text-gray-800">
                            {page.structuredHints.map((h) => (
                              <li key={`${h.source}|${h.field}|${h.value}`}>
                                {FIELD_NAMES[h.field]}: <span className="font-medium">{shortValue(h.value, 120)}</span>{' '}
                                <span className="text-gray-500">({h.reason})</span>
                              </li>
                            ))}
                          </ul>
                          <p className="mt-1 text-gray-600">
                            {page.structuredSupported
                              ? 'Pick them in the rows above: they are read from the page’s own data, which survives a redesign of the page.'
                              : 'Update the Browser tool to read these directly; until then use them to check your picks.'}
                          </p>
                        </div>
                      )}
                      {hasView && (
                        <div className="space-y-2 border-t border-gray-200 pt-3" data-testid="job-detail-description-box">
                          <div className="font-semibold text-gray-900">The description</div>
                          {renderDescription(page)}
                        </div>
                      )}
                    </div>
                  )}
                  {hasView && (
                    <DetailPageView
                      snapshotId={snapshotId}
                      view={view}
                      picks={picks}
                      rule={rule}
                      large={!fieldsShown}
                      reveal={reveal}
                      onAnswer={answerFromPage}
                    />
                  )}
                </div>
                {!page.snapshot && page.snapshotError && (
                  <p className="text-xs text-gray-500" data-testid="job-detail-page-unavailable">
                    The page itself cannot be shown here: {page.snapshotError}. The rows work as before.
                  </p>
                )}
                {page.snapshot && !hasView && page.snapshotError === undefined && (
                  <p className="text-xs text-gray-500" data-testid="job-detail-page-unavailable">
                    The page has no element to click. The rows work as before.
                  </p>
                )}
              </InstructionSection>

              {!hasView && (
                <InstructionSection
                  number={3}
                  title="The description"
                  description="Pick the box that holds it, or read it with a rule: several boxes, things to leave out, headings, a longer limit."
                >
                  {renderDescription(page)}
                </InstructionSection>
              )}

              <InstructionSection
                number={hasView && !fieldsShown ? 3 : 4}
                title="Check and save"
                description="See the instructions these picks make, check them on sample jobs of this link, then save."
              >
                <div className="rounded-xl border border-gray-200 bg-white p-3" data-testid="job-detail-pick-preview">
                  <div className="font-semibold text-gray-900">The instructions these picks make</div>
                  {!canPreview(picks, rule) && (
                    <p className="mt-1 text-gray-600">Choose where the description is to see the instructions.</p>
                  )}
                  {previewError && (
                    <p role="alert" className="mt-1 text-red-700">
                      {previewError}
                    </p>
                  )}
                  {preview && !preview.valid && <p className="mt-1 text-red-700">{preview.error}</p>}
                  {preview?.valid && preview.yaml && (
                    <pre className="mt-1 max-h-48 overflow-auto whitespace-pre-wrap rounded-md bg-gray-900 p-2 font-mono text-[11px] text-gray-100">
                      {preview.yaml}
                    </pre>
                  )}
                </div>

                {canPreview(picks, rule) && (
                  <VerifyPanel
                    linkId={link.id}
                    page={page}
                    choices={toChoices(picks, rule, replaceTitle)}
                    previewValid={preview?.valid === true}
                    onSaved={saved}
                  />
                )}
              </InstructionSection>
            </>
          )}
        </div>
      )}
    </div>
  )
}
