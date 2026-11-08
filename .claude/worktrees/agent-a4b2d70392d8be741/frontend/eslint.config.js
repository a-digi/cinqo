import js from '@eslint/js'
import tseslint from 'typescript-eslint'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import jsxA11y from 'eslint-plugin-jsx-a11y'
import eslintConfigPrettier from 'eslint-config-prettier'

// eslint-plugin-react-hooks v7 ships its own "recommended-latest" config
// bundling ~15 React Compiler static-analysis rules (immutability, purity,
// set-state-in-render, etc.) alongside the classic two. Deliberately NOT
// adopted wholesale here — this is a first-time lint rollout on a
// never-linted codebase, and those compiler rules are a separate, much
// larger decision (see plan/ai/frontend/frontend/step-14-lint-and-code-quality-gates.md).
// Only the two rules every hooks-using React codebase should have are
// enabled explicitly.
export default tseslint.config(
  { ignores: ['dist/**', 'node_modules/**'] },
  {
    files: ['**/*.{ts,tsx}'],
    extends: [
      js.configs.recommended,
      ...tseslint.configs.strictTypeChecked,
      ...tseslint.configs.stylisticTypeChecked,
      jsxA11y.flatConfigs.recommended,
      reactRefresh.configs.vite,
    ],
    languageOptions: {
      parserOptions: {
        project: ['./tsconfig.app.json', './tsconfig.node.json'],
        tsconfigRootDir: import.meta.dirname,
      },
    },
    plugins: {
      'react-hooks': reactHooks,
    },
    rules: {
      'react-hooks/rules-of-hooks': 'error',
      'react-hooks/exhaustive-deps': 'warn',
      // A number in a template literal (`${count} items`) is safe and
      // idiomatic — strictTypeChecked's default here exists to catch
      // objects/arrays stringifying as "[object Object]", not this.
      '@typescript-eslint/restrict-template-expressions': ['error', { allowNumber: true }],
    },
  },
  eslintConfigPrettier,
)
