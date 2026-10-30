import type { WidgetDefinition } from '../../api/dashboard'
import { ChartWidget } from './widgets/ChartWidget'
import { CustomWidget } from './widgets/CustomWidget'
import { ListWidget } from './widgets/ListWidget'
import { StatWidget } from './widgets/StatWidget'
import { IconBadge } from './widgetIcons'
import { stateMessages, unsupportedMessage } from './widgetDisplay'
import { parseWidgetData } from './widgetKinds'
import { widgetTheme, type Accent, type DecorShape } from './widgetTheme'
import { useWidgetData, type WidgetResult } from './useWidgetData'

function Message({ text, onRetry, accent }: { text: string; onRetry?: () => void; accent: Accent }) {
  return (
    <div className="flex h-full flex-col items-start justify-center gap-2">
      <p className="text-sm text-gray-500">{text}</p>
      {onRetry && (
        <button
          type="button"
          onClick={onRetry}
          className="rounded-full px-3 py-1 text-xs font-medium"
          style={{ background: accent.soft, color: accent.text }}
        >
          Retry
        </button>
      )}
    </div>
  )
}

function Skeleton({ accent }: { accent: Accent }) {
  const bar = { background: accent.soft }
  return (
    <div className="flex h-full animate-pulse flex-col justify-center gap-3" aria-label="Loading">
      <div className="h-3 w-1/3 rounded-full" style={bar} />
      <div className="h-8 w-1/2 rounded-2xl" style={bar} />
      <div className="h-3 w-2/3 rounded-full" style={bar} />
    </div>
  )
}

// Soft shapes tucked behind the content, in the accent colour. Each kind gets its
// own so the cards do not all look alike.
function Decor({ shape, accent }: { shape: DecorShape; accent: Accent }) {
  const fill = (opacity: number) => ({ background: `linear-gradient(135deg, ${accent.from}, ${accent.to})`, opacity })
  if (shape === 'diamond') {
    return (
      <>
        <div className="pointer-events-none absolute -right-8 -top-8 h-28 w-28 rotate-45 rounded-3xl" style={fill(0.12)} />
        <div className="pointer-events-none absolute right-10 top-12 h-8 w-8 rotate-45 rounded-lg" style={fill(0.16)} />
      </>
    )
  }
  if (shape === 'ring') {
    return (
      <>
        <div
          className="pointer-events-none absolute -right-10 -top-10 h-32 w-32 rounded-full border-[14px]"
          style={{ borderColor: accent.from, opacity: 0.14 }}
        />
        <div className="pointer-events-none absolute -bottom-6 -left-6 h-20 w-20 rounded-full" style={fill(0.1)} />
      </>
    )
  }
  return (
    <>
      <div className="pointer-events-none absolute -right-6 -top-6 h-28 w-28 rounded-full" style={fill(0.14)} />
      <div className="pointer-events-none absolute right-14 top-16 h-10 w-10 rounded-full" style={fill(0.2)} />
    </>
  )
}

function Body({ widget, result, retry, accent }: { widget: WidgetDefinition; result: WidgetResult; retry: () => void; accent: Accent }) {
  if (result.status === 'loading') return <Skeleton accent={accent} />
  if (result.status === 'failed') return <Message text={stateMessages.error} onRetry={retry} accent={accent} />

  const { data } = result
  if (data.state === 'unavailable' || data.state === 'error') {
    return <Message text={stateMessages[data.state]} onRetry={retry} accent={accent} />
  }
  if (data.state === 'empty') return <Message text={stateMessages.empty} accent={accent} />

  // The payload was validated by the backend; this re-check keeps a bug or a
  // version skew from crashing a renderer.
  switch (widget.kind) {
    case 'stat': {
      const parsed = parseWidgetData('stat', data.schemaVersion, data.data)
      return parsed ? <StatWidget data={parsed} accent={accent} /> : <Message text={unsupportedMessage} accent={accent} />
    }
    case 'list': {
      const parsed = parseWidgetData('list', data.schemaVersion, data.data)
      return parsed ? <ListWidget data={parsed} accent={accent} /> : <Message text={unsupportedMessage} accent={accent} />
    }
    case 'chart': {
      const parsed = parseWidgetData('chart', data.schemaVersion, data.data)
      return parsed ? (
        <ChartWidget data={parsed} accent={accent} gradientId={`g-${widget.id}`} />
      ) : (
        <Message text={unsupportedMessage} accent={accent} />
      )
    }
    case 'custom': {
      const parsed = parseWidgetData('custom', data.schemaVersion, data.data)
      return parsed ? <CustomWidget widgetId={widget.id} data={parsed} /> : <Message text={unsupportedMessage} accent={accent} />
    }
  }
}

// One widget on the dashboard: a rounded, softly tinted card with a coloured icon
// badge and decorative shapes, that loads its own data and shows a skeleton, the
// data, an empty note, or an error with Retry. The slide-in is the shared
// cinqo-card-enter animation (the same one Card's `animate` uses).
export function WidgetFrame({ widget, animationDelayMs }: { widget: WidgetDefinition; animationDelayMs: number }) {
  const { result, retry } = useWidgetData(widget.id, widget.refreshSeconds)
  const theme = widgetTheme(widget)
  const { accent } = theme
  return (
    <div
      className="cinqo-card-enter relative flex h-full flex-col overflow-hidden rounded-3xl border p-5 shadow-sm transition-shadow hover:shadow-lg"
      style={{
        animationDelay: animationDelayMs > 0 ? `${animationDelayMs}ms` : undefined,
        borderColor: accent.soft,
        background: `linear-gradient(160deg, #ffffff 40%, ${accent.soft} 100%)`,
      }}
    >
      <Decor shape={theme.shape} accent={accent} />
      <div className="relative mb-4 flex items-center gap-3">
        <IconBadge name={theme.icon} from={accent.from} to={accent.to} />
        <h2 className="min-w-0 truncate text-sm font-semibold text-gray-900" title={widget.description || undefined}>
          {widget.title}
        </h2>
      </div>
      <div className="relative min-h-0 flex-1">
        <Body widget={widget} result={result} retry={retry} accent={accent} />
      </div>
    </div>
  )
}
