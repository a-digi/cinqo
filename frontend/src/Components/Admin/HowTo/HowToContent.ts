import { isKnownPageId, type HowToPageId } from './howToRules'

// The words of the How To landing pages (plan/ai/frontend/step-219-how-to-landing-pages.md). Editorial text, kept here (not in the Go registry) because
// it changes with the providers' offers. Pure data, no React, so the rules can be script-checked: every page has every section, and no text names a
// price or promises free credit (nothing in the code can verify one: the text sends people to the provider). What is live on a page (the models, the
// keys added, whether the local agent is on, the buttons) is not in here: the page reads it from the API.

export interface HowToTile {
  title: string
  text: string
}

export interface HowToStep {
  title: string
  text: string
}

export interface HowToQuestion {
  question: string
  answer: string
}

export interface HowToPageContent {
  id: HowToPageId
  // What the menu and the slide's "Read more" say.
  label: string
  eyebrow: string
  title: string
  // The one line under the title.
  promise: string
  glance: HowToTile[]
  goodFor: string[]
  notSoGood: string[]
  steps: HowToStep[]
  cost: string
  data: string
  questions: HowToQuestion[]
}

const KEY_STORED =
  'Your key is encrypted on your own server and is only ever shown masked. Cinqo uses it to talk to the provider for your conversations and for nothing else.'

export const HOW_TO_CONTENT: Record<HowToPageId, HowToPageContent> = {
  overview: {
    id: 'overview',
    label: 'Overview',
    eyebrow: 'How AI works in Cinqo',
    title: 'Bring your own AI, free or premium.',
    promise:
      'A platform is the company whose models answer your conversations. You connect one with your own key, or run a small model on your own computer.',
    glance: [
      {
        title: 'Two ways to get answers',
        text: 'A remote platform (OpenRouter, OpenAI, Anthropic), reached with your own API key, or a local agent that runs on this computer.',
      },
      {
        title: 'Free or premium',
        text: 'Some models are free to use, with limits. Premium models are billed by the provider, per use, to your own account with them.',
      },
      {
        title: 'Your key stays on your server',
        text: 'Keys are encrypted at rest and only shown masked. Nothing is shared with Cinqo or anyone else.',
      },
      {
        title: 'You choose',
        text: 'You pick the platform and model for a conversation. Tools such as Career offer the ones that are ready, and use a ready local agent before a remote platform when you have not chosen.',
      },
    ],
    goodFor: [
      'Getting started quickly: one key, one platform, and conversations work.',
      'Trying Cinqo without paying: OpenRouter offers free models, and the local agent costs nothing per use.',
      'Keeping control: you can add, replace or remove a key at any time on the Remote Platforms page.',
    ],
    notSoGood: [
      'A platform you have not added a key for cannot answer: it is listed, but not usable until a key is added.',
      'Prices and limits belong to the providers and change: check their sites before you rely on a model.',
    ],
    steps: [
      {
        title: 'Pick a platform',
        text: 'Read the pages for OpenRouter, OpenAI and Anthropic, or the local agent, and choose what suits you.',
      },
      {
        title: 'Get a key from the provider',
        text: "Each page links to the provider's own page where the key is created. You need an account with them.",
      },
      {
        title: 'Add the key in Cinqo',
        text: 'Open Remote Platforms, choose the platform and paste the key. It is stored encrypted and shown masked from then on.',
      },
      {
        title: 'Use it',
        text: 'Start a conversation and choose the platform and model, or let a tool such as Career offer what is ready.',
      },
    ],
    cost: 'Cinqo itself does not bill you for conversations. A premium model is billed by its provider to your account with them, per use. Free models and the local agent have no cost per use. Providers decide their prices, limits and free offers and can change them: check their sites for what is true today.',
    data:
      KEY_STORED + ' A conversation on a remote platform is sent to that provider. A conversation on the local agent is not sent anywhere.',
    questions: [
      {
        question: 'Do I need a key to try Cinqo?',
        answer: 'Not if you run the local agent on your own computer. For a remote platform you need a key from that provider.',
      },
      { question: 'Can I use more than one platform?', answer: 'Yes. Add a key for each one you want and choose per conversation.' },
      { question: 'Where do I add or remove a key?', answer: 'On the Remote Platforms page in the AI menu.' },
      {
        question: 'What is the difference between free and premium?',
        answer:
          'Free models cost nothing per use but can have limits and can change over time. Premium models are billed per use by the provider.',
      },
    ],
  },
  'local-agents': {
    id: 'local-agents',
    label: 'Local Agents',
    eyebrow: 'On this computer',
    title: 'Your own AI, on your own machine.',
    promise:
      'A small language model answers your conversations without a cloud provider. Nothing is sent to one, and nothing is billed per use.',
    glance: [
      { title: 'Runs here', text: "Cinqo downloads and runs the model on this computer. It uses this machine's memory and processor." },
      { title: 'Private', text: 'Your conversations on a local agent are not sent to a cloud provider.' },
      {
        title: 'No cost per use',
        text: "There is no bill for a conversation. The one-time cost is the download and the computer's own resources.",
      },
      { title: 'Simpler answers', text: 'A small model answers slower and simpler than the cloud models, and it has no sub-agents.' },
    ],
    goodFor: [
      'Private, simple questions and drafts.',
      'Using Cinqo without any account or key.',
      'Tasks in a tool that names its few tools: a model marked as able to use tools gets exactly those.',
    ],
    notSoGood: [
      'Long, multi-step work: a small model loses the thread sooner than a cloud model.',
      'Tasks that need a lot of text at once: the model reads only as much as its context size allows, and a long conversation can exceed it.',
      'A computer with little memory: a model that does not fit is marked as such and should not be used.',
    ],
    steps: [
      {
        title: 'Open Local Agents',
        text: 'It is in the AI menu. The page shows which models Cinqo manages and whether each fits this computer.',
      },
      { title: 'Choose a model', text: 'Pick the default model. Tools can choose any other model you have downloaded.' },
      {
        title: 'Switch it on',
        text: 'Cinqo downloads what it needs (the runtime and the model) in the background. It shows as on only when it is really ready.',
      },
      {
        title: 'Use it',
        text: 'It shows up as a platform when you start a conversation, and a tool such as Career offers it when it is ready.',
      },
    ],
    cost: "There is no cost per use. Downloading the runtime and a model takes disk space and some time, and running it uses this computer's memory and processor while it answers.",
    data: "Nothing you write to a local agent is sent to a cloud provider. The model runs on this computer, and its downloaded files stay in Cinqo's own data folder.",
    questions: [
      {
        question: 'Which computers are supported?',
        answer:
          'It is being built and tested on macOS with Apple Silicon and on Linux (amd64) first. Windows is planned. The Local Agents page says whether this computer is supported.',
      },
      {
        question: 'Why is a model marked "needs a download"?',
        answer: 'Cinqo has not finished downloading what it needs, or a file was removed. Continue the download on the Local Agents page.',
      },
      {
        question: 'Can I use it for importing a CV?',
        answer: 'Only with a model that manages it: the smallest model was measured not to, and the page says so for each model.',
      },
      {
        question: 'Can I switch it off?',
        answer: 'Yes, on the Local Agents page. Downloaded files can be removed once it is off and no download is running.',
      },
    ],
  },
  openrouter: {
    id: 'openrouter',
    label: 'OpenRouter',
    eyebrow: 'Platform',
    title: 'One key, many models, including free ones.',
    promise: 'OpenRouter reaches several providers with a single key, and its free router picks a free model for each request.',
    glance: [
      { title: 'One key', text: 'A single OpenRouter key gives access to models from several providers.' },
      { title: 'Free and premium', text: 'The free router answers with a free model. Premium models are billed per use.' },
      { title: 'Limits apply', text: 'Free models can have limits and change over time. OpenRouter says what is available now.' },
      { title: 'Sent to OpenRouter', text: "Conversations on this platform go to OpenRouter, and from there to the model's provider." },
    ],
    goodFor: [
      'Trying Cinqo at no cost.',
      'Reaching several providers with one key and one bill.',
      'Comparing models without opening an account with each provider.',
    ],
    notSoGood: [
      'Work that needs the same model every time: the free router picks a free model for each request, and which one can change.',
      'Anything that depends on a free model staying available: free models can have limits and can disappear.',
    ],
    steps: [
      {
        title: 'Create a key at OpenRouter',
        text: 'Open the OpenRouter keys page with the link on this page and create a key in your own account.',
      },
      { title: 'Add it in Cinqo', text: 'Open Remote Platforms, choose OpenRouter and paste the key.' },
      { title: 'Choose a model', text: 'Pick the free models router, or one of the premium models listed on this page.' },
      { title: 'Start a conversation', text: 'Choose OpenRouter as the platform when you start one.' },
    ],
    cost: "OpenRouter's free router picks a free model for each request. Free models can have limits and can change over time. Premium models are billed per use by OpenRouter, to your own account there. Check OpenRouter for what is available and what it costs now.",
    data: KEY_STORED + ' What you write in a conversation is sent to OpenRouter and on to the provider of the model that answers.',
    questions: [
      {
        question: 'Do I need to pay to use OpenRouter?',
        answer:
          "Not for the free models, within their limits. Premium models are billed per use. OpenRouter's site says what is available now.",
      },
      {
        question: 'Which model answers when I use the free router?',
        answer: 'The router picks a free model for each request, so it can differ from one answer to the next.',
      },
      {
        question: 'Why did a free model stop working?',
        answer: 'Free models can have limits and can change over time. Choose another model, or check OpenRouter.',
      },
    ],
  },
  openai: {
    id: 'openai',
    label: 'OpenAI',
    eyebrow: 'Platform',
    title: 'GPT models, billed per use.',
    promise: "OpenAI's GPT models answer your conversations, through your own OpenAI account.",
    glance: [
      { title: 'GPT models', text: "OpenAI's models, as many as Cinqo offers for this platform, listed on this page." },
      { title: 'Billed per use', text: 'You pay OpenAI for what you use, through your own account.' },
      { title: 'Broad skills', text: 'General-purpose work: writing, analysis, coding and tool use.' },
      { title: 'Sent to OpenAI', text: 'Conversations on this platform go to OpenAI.' },
    ],
    goodFor: [
      'Broad, general-purpose work: writing, analysis, coding and tool use.',
      'Tools that call functions: GPT models can use the tools a task names.',
    ],
    notSoGood: [
      'Working with no account at all: this platform needs an OpenAI account and a key.',
      'Private material that must not leave your computer: use the local agent for that.',
    ],
    steps: [
      {
        title: 'Create a key at OpenAI',
        text: 'Open the OpenAI keys page with the link on this page and create a key in your own account.',
      },
      {
        title: 'Check billing with OpenAI',
        text: 'Use of a premium model is billed by OpenAI to your account: check their site for current pricing.',
      },
      { title: 'Add the key in Cinqo', text: 'Open Remote Platforms, choose OpenAI and paste the key.' },
      { title: 'Start a conversation', text: 'Choose OpenAI and a model when you start one.' },
    ],
    cost: 'You pay OpenAI for what you use, through your own OpenAI account. Check their site for current pricing.',
    data: KEY_STORED + ' What you write in a conversation is sent to OpenAI.',
    questions: [
      { question: 'Does Cinqo charge me for OpenAI use?', answer: 'No. OpenAI bills you directly for what you use.' },
      { question: 'Where is my key shown?', answer: 'Only masked, on the Remote Platforms page. It cannot be read back.' },
      {
        question: 'Can I remove the key?',
        answer: 'Yes, on the Remote Platforms page. The platform then stops being usable until a key is added again.',
      },
    ],
  },
  anthropic: {
    id: 'anthropic',
    label: 'Anthropic',
    eyebrow: 'Platform',
    title: 'Claude models, billed per use.',
    promise: "Anthropic's Claude models answer your conversations, through your own Anthropic account.",
    glance: [
      { title: 'Claude models', text: "Anthropic's models, as many as Cinqo offers for this platform, listed on this page." },
      { title: 'Billed per use', text: 'You pay Anthropic for what you use, through your own account.' },
      { title: 'Careful reasoning', text: 'Long multi-step tasks and coding.' },
      { title: 'Sent to Anthropic', text: 'Conversations on this platform go to Anthropic.' },
    ],
    goodFor: [
      'Careful reasoning, long multi-step tasks and coding.',
      'Tools that call functions: Claude models can use the tools a task names.',
    ],
    notSoGood: [
      'Working with no account at all: this platform needs an Anthropic account and a key.',
      'Private material that must not leave your computer: use the local agent for that.',
    ],
    steps: [
      {
        title: 'Create a key at Anthropic',
        text: 'Open the Anthropic console with the link on this page and create a key in your own account.',
      },
      {
        title: 'Check billing with Anthropic',
        text: 'Use of a model is billed by Anthropic to your account: check their site for current pricing.',
      },
      { title: 'Add the key in Cinqo', text: 'Open Remote Platforms, choose Anthropic and paste the key.' },
      { title: 'Start a conversation', text: 'Choose Anthropic and a model when you start one.' },
    ],
    cost: 'You pay Anthropic for what you use, through your own Anthropic account. Check their site for current pricing.',
    data: KEY_STORED + ' What you write in a conversation is sent to Anthropic.',
    questions: [
      { question: 'Does Cinqo charge me for Anthropic use?', answer: 'No. Anthropic bills you directly for what you use.' },
      { question: 'Where is my key shown?', answer: 'Only masked, on the Remote Platforms page. It cannot be read back.' },
      {
        question: 'Can I remove the key?',
        answer: 'Yes, on the Remote Platforms page. The platform then stops being usable until a key is added again.',
      },
    ],
  },
}

// The page of a platform this file does not know (one added to the registry later): the same sections, from what the platform list says.
export function genericContent(platformId: string, platformName: string): HowToPageContent {
  return {
    id: platformId as HowToPageId,
    label: platformName,
    eyebrow: 'Platform',
    title: `${platformName}.`,
    promise: `AI models from ${platformName}, reached with your own key.`,
    glance: [
      { title: 'Your own key', text: `Add an API key from ${platformName} and conversations can use its models.` },
      { title: 'Billed by the provider', text: `${platformName} decides what its models cost: check their site.` },
      { title: 'Sent to the provider', text: `Conversations on this platform go to ${platformName}.` },
    ],
    goodFor: [`Conversations answered by ${platformName}'s models.`],
    notSoGood: ['Private material that must not leave your computer: use the local agent for that.'],
    steps: [
      { title: `Get a key from ${platformName}`, text: 'Create it in your own account with them.' },
      { title: 'Add the key in Cinqo', text: `Open Remote Platforms, choose ${platformName} and paste the key.` },
      { title: 'Start a conversation', text: `Choose ${platformName} and a model when you start one.` },
    ],
    cost: `${platformName} decides what its models cost and what is free. Check their site for what is true today.`,
    data: KEY_STORED + ` What you write in a conversation is sent to ${platformName}.`,
    questions: [{ question: 'Where is my key shown?', answer: 'Only masked, on the Remote Platforms page. It cannot be read back.' }],
  }
}

export function contentFor(pageId: string, platformName?: string): HowToPageContent | null {
  // the list of ids, not `in`: a page id such as "constructor" would otherwise find a property every object has
  if (isKnownPageId(pageId)) return HOW_TO_CONTENT[pageId]
  return platformName ? genericContent(pageId, platformName) : null
}
