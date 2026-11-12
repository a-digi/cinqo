// The shapes every "How it works" infographic is made of (plan 262, shared by plan 266): a chapter, its captions and links, and one step of the journey.
// Pure types, no React.

export interface Caption {
  title: string
  text: string
}

export interface InfoLink {
  label: string
  path: string
}

export interface Chapter<Id extends string = string> {
  id: Id
  // The short name on the chapter bar.
  nav: string
  title: string
  // One sentence: what the chapter shows.
  promise: string
  captions: Caption[]
  // The real buttons to click, exactly as they are named on the page.
  click: string[]
  links: InfoLink[]
}

export interface JourneyStep {
  id: string
  // null: the page cannot tell (choosing the AI, looking at the jobs); drawn without a done mark.
  done: boolean | null
}
