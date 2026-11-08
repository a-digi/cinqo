// Who made a tool or a repository, as the publisher states it (plan/ai/tools/repository/tool-metadata.md). Every
// one of these is a claim by the publisher, not proof of identity, and every address is only ever shown as a link.

export interface ToolAuthor {
  name: string
  email?: string
  url?: string
}

export interface ToolCompany {
  name: string
  url?: string
}

export interface ToolLinks {
  homepage?: string
  documentation?: string
  source?: string
  issues?: string
  support?: string
  security?: string
}

// One tool version's attribution: an installed tool's (what its own package declared) or a repository's entry.
export interface ToolMetadata {
  authors?: ToolAuthor[]
  company?: ToolCompany
  license?: string
  links?: ToolLinks
  copyright?: string
  category?: string
  keywords?: string[]
}

// Who maintains a repository.
export interface Publisher {
  authors: ToolAuthor[]
  company?: ToolCompany
  links?: ToolLinks
  generatedAt?: string
}
