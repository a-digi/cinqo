import { useEffect, useState } from 'react'

interface TypewriterProps {
  text: string
  // How often a fresh typing cycle starts, in ms — the text is
  // cleared and retyped from scratch every cycleMs, holding the fully
  // typed string for whatever's left of the cycle once typing itself
  // finishes. Default matches this component's first caller
  // (CrawlPanel's "Check Progress - AI").
  cycleMs?: number
  // Delay between each revealed character, in ms.
  typingSpeedMs?: number
  className?: string
}

// Shared, controlled typewriter effect — text-in, no API calls, no
// state beyond its own animation. Loops for as long as it stays
// mounted; the caller decides when to show/hide it (e.g. CrawlPanel
// only renders this while a "Generate with AI" run is in flight).
// See plan/ai/tools/career/step-60-generate-with-ai-live-chat-window.md.
export function Typewriter({ text, cycleMs = 15000, typingSpeedMs = 50, className }: TypewriterProps) {
  const [display, setDisplay] = useState('')

  useEffect(() => {
    let charIndex = 0
    let charTimer: ReturnType<typeof setInterval> | undefined

    function startTyping() {
      charIndex = 0
      setDisplay('')
      clearInterval(charTimer)
      charTimer = setInterval(() => {
        charIndex += 1
        setDisplay(text.slice(0, charIndex))
        if (charIndex >= text.length) {
          clearInterval(charTimer)
        }
      }, typingSpeedMs)
    }

    startTyping()
    const cycleTimer = setInterval(startTyping, cycleMs)

    return () => {
      clearInterval(charTimer)
      clearInterval(cycleTimer)
    }
  }, [text, cycleMs, typingSpeedMs])

  return (
    <span className={className}>
      {/* The animated reveal is purely decorative — a screen reader
          announcing every intermediate character as it types would be
          unusable, so it's hidden and a plain, static, complete label
          takes its place in the accessibility tree instead. */}
      <span aria-hidden="true">
        {display}
        <span className="animate-pulse">▍</span>
      </span>
      <span className="sr-only">{text}</span>
    </span>
  )
}
