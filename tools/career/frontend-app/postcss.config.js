import tailwindcss from '@tailwindcss/postcss'
import autoprefixer from 'autoprefixer'
// Keeps `lg:`/`hover:`/... variants working when several tools' Tailwind stylesheets share one page (see the file's header).
import layerVariants from '../../../build-shared/postcss-layer-variants.cjs'

export default {
  plugins: [tailwindcss(), autoprefixer(), layerVariants()],
}
