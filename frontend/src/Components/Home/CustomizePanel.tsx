import { IconBadge } from './widgetIcons'
import { widgetTheme } from './widgetTheme'
import { IconButton } from '../../Shared/Components/IconButton/IconButton'
import { ArrowDownIcon, ArrowUpIcon, EyeIcon, EyeOffIcon } from '../../Shared/Components/IconButton/icons'
import { Pill } from '../../Shared/Components/Pill/Pill'
import type { WidgetDefinition } from '../../api/dashboard'

export interface CustomizePanelProps {
  widgets: WidgetDefinition[]
  disabled: boolean
  onMove: (id: string, direction: -1 | 1) => void
  onToggle: (id: string) => void
}

// The Customize view: every widget this user may see, in their order, each with
// move up/down and show/hide. Changes are a draft until the page saves them.
export function CustomizePanel({ widgets, disabled, onMove, onToggle }: CustomizePanelProps) {
  return (
    <ul className="divide-y divide-gray-100 rounded-xl border border-gray-200 bg-white shadow-sm">
      {widgets.map((widget, index) => {
        const theme = widgetTheme(widget)
        return (
          <li key={widget.id} className={`flex items-center gap-3 px-4 py-3 ${widget.hidden ? 'opacity-60' : ''}`}>
            <IconBadge name={theme.icon} from={theme.accent.from} to={theme.accent.to} size="sm" />
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium text-gray-900">{widget.title}</p>
              {widget.description && <p className="truncate text-xs text-gray-500">{widget.description}</p>}
            </div>
            <Pill outline>{widget.provider}</Pill>
            <div className="flex shrink-0 items-center gap-1">
              <IconButton
                icon={<ArrowUpIcon />}
                label={`Move ${widget.title} up`}
                onClick={() => {
                  onMove(widget.id, -1)
                }}
                disabled={disabled || index === 0}
              />
              <IconButton
                icon={<ArrowDownIcon />}
                label={`Move ${widget.title} down`}
                onClick={() => {
                  onMove(widget.id, 1)
                }}
                disabled={disabled || index === widgets.length - 1}
              />
              <IconButton
                icon={widget.hidden ? <EyeOffIcon /> : <EyeIcon />}
                label={widget.hidden ? `Show ${widget.title}` : `Hide ${widget.title}`}
                onClick={() => {
                  onToggle(widget.id)
                }}
                disabled={disabled}
              />
            </div>
          </li>
        )
      })}
    </ul>
  )
}
