// Small outline icons for the resource bar (16 px, drawn with the text colour). Decorative: the chips carry their own text and labels.
interface IconProps {
  className?: string
}

function Svg({ className, children }: IconProps & { children: React.ReactNode }) {
  return (
    <svg
      viewBox="0 0 20 20"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      className={className ?? 'h-4 w-4'}
    >
      {children}
    </svg>
  )
}

export const LaptopIcon = (p: IconProps) => (
  <Svg {...p}>
    <rect x="4" y="4.5" width="12" height="8" rx="1.2" />
    <path d="M2.5 15.5h15" />
  </Svg>
)

export const MemoryIcon = (p: IconProps) => (
  <Svg {...p}>
    <rect x="2.5" y="6" width="15" height="7" rx="1" />
    <path d="M5.5 13v2M8.5 13v2M11.5 13v2M14.5 13v2M6 8.8h3M11 8.8h3" />
  </Svg>
)

export const CpuIcon = (p: IconProps) => (
  <Svg {...p}>
    <rect x="5.5" y="5.5" width="9" height="9" rx="1.2" />
    <rect x="8.2" y="8.2" width="3.6" height="3.6" rx="0.5" />
    <path d="M8 3v2.5M12 3v2.5M8 14.5V17M12 14.5V17M3 8h2.5M3 12h2.5M14.5 8H17M14.5 12H17" />
  </Svg>
)

export const DiskIcon = (p: IconProps) => (
  <Svg {...p}>
    <ellipse cx="10" cy="5.5" rx="6" ry="2.5" />
    <path d="M4 5.5v9c0 1.4 2.7 2.5 6 2.5s6-1.1 6-2.5v-9M4 10c0 1.4 2.7 2.5 6 2.5s6-1.1 6-2.5" />
  </Svg>
)

export const SwapIcon = (p: IconProps) => (
  <Svg {...p}>
    <path d="M4 7.5h11l-3-3M16 12.5H5l3 3" />
  </Svg>
)

export const CubeIcon = (p: IconProps) => (
  <Svg {...p}>
    <path d="M10 2.8 16.5 6.4v7.2L10 17.2 3.5 13.6V6.4L10 2.8ZM10 10v7M3.7 6.5 10 10l6.3-3.5" />
  </Svg>
)

export const BotIcon = (p: IconProps) => (
  <Svg {...p}>
    <rect x="4" y="6.5" width="12" height="9" rx="2.2" />
    <path d="M10 6.5V3.8M7.6 10.8v.4M12.4 10.8v.4M8 13.2h4" />
    <circle cx="10" cy="3.3" r="0.8" />
  </Svg>
)

export const AlertIcon = (p: IconProps) => (
  <Svg {...p}>
    <path d="M10 3.2 17.3 16H2.7L10 3.2ZM10 8v3.6M10 13.6v.2" />
  </Svg>
)

export const CheckIcon = (p: IconProps) => (
  <Svg {...p}>
    <circle cx="10" cy="10" r="7" />
    <path d="m6.8 10.2 2.2 2.2 4.2-4.6" />
  </Svg>
)

export const EjectIcon = (p: IconProps) => (
  <Svg {...p} className={p.className ?? 'h-3 w-3 opacity-70'}>
    <path d="M10 4.5 16 11H4l6-6.5ZM4.5 14.5h11" />
  </Svg>
)
