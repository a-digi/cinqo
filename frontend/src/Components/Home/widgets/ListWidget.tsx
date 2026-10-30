import { Link } from 'react-router-dom'
import { Pill } from '../../../Shared/Components/Pill/Pill'
import { safeLink, type ListData, type ListItem } from '../widgetKinds'
import { initialOf, type Accent } from '../widgetTheme'

function Row({ item, accent }: { item: ListItem; accent: Accent }) {
  const link = safeLink(item.link)
  const content = (
    <div className="flex items-center gap-3 py-2">
      <span
        className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-sm font-bold"
        style={{
          background: `linear-gradient(135deg, ${accent.soft}, #ffffff)`,
          color: accent.text,
          boxShadow: `inset 0 0 0 1.5px ${accent.from}`,
        }}
      >
        {initialOf(item.title)}
      </span>
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium text-gray-900">{item.title}</p>
        {item.subtitle && <p className="truncate text-xs text-gray-500">{item.subtitle}</p>}
      </div>
      <div className="flex shrink-0 items-center gap-2">
        {item.badge && <Pill variant={item.badge.variant}>{item.badge.text}</Pill>}
        {item.meta && (
          <span className="rounded-full px-2 py-0.5 text-xs" style={{ background: accent.soft, color: accent.text }}>
            {item.meta}
          </span>
        )}
      </div>
    </div>
  )
  return link ? (
    <Link to={link} className="block rounded-2xl px-2 transition-colors hover:bg-white/70">
      {content}
    </Link>
  ) : (
    <div className="px-2">{content}</div>
  )
}

export function ListWidget({ data, accent }: { data: ListData; accent: Accent }) {
  return (
    <ul className="-mx-2">
      {data.items.map((item, i) => (
        <li key={`${i}-${item.title}`}>
          <Row item={item} accent={accent} />
        </li>
      ))}
    </ul>
  )
}
