import { createElement } from 'react'
import type { SliderSlide } from '../../../Shared/Components/Slider/Slider'
import { LocalAiSlide } from './LocalAiSlide'

// The Local AI slide, handed to the Platforms page's slider by one line in HowTo/HowToPage.tsx. A .ts file with no JSX so
// that the only thing exported next to a component is data, which the fast-refresh lint rule accepts.
export const localAiSlide: SliderSlide = { id: 'local-ai', label: 'Local Agents', content: createElement(LocalAiSlide) }
