// Keeps `lg:`/`hover:`/... variants working when the tools' Tailwind stylesheets share this page (see build-shared/postcss-layer-variants.cjs).
module.exports = {
  plugins: [require('@tailwindcss/postcss')(), require('autoprefixer')(), require('../build-shared/postcss-layer-variants.cjs')()],
}
