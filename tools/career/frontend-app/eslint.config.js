import js from '@eslint/js'
import tseslint from 'typescript-eslint'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import jsxA11y from 'eslint-plugin-jsx-a11y'
import eslintConfigPrettier from 'eslint-config-prettier'

// Same shape as the main Cinqo frontend's own eslint.config.js — see
// plan/ai/frontend/frontend/step-14-lint-and-code-quality-gates.md.
// eslint-plugin-react-hooks v7's own "recommended-latest" config bundles
// ~15 React Compiler static-analysis rules alongside the classic two;
// deliberately not adopted wholesale here for the same reason as the
// main frontend — only rules-of-hooks/exhaustive-deps are enabled
// explicitly.
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
        project: ['./tsconfig.json'],
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
  // Plan 192: the instruction engines are decoupled. An engine folder imports nothing of another engine (only registry.ts names them), and
  // the shared screen code reaches an engine through the registry, never by its folder.
  ...['rod', 'katana'].map((id) => ({
    files: [`src/Crawler/engines/${id}/**/*.{ts,tsx}`],
    rules: {
      'no-restricted-imports': [
        'error',
        {
          patterns: [
            {
              group: ['../rod', '../rod/*', '../katana', '../katana/*', '**/engines/rod/**', '**/engines/katana/**'].filter(
                (g) => !g.includes(`/${id}`) && !g.endsWith(`${id}`) && !g.endsWith(`${id}/*`),
              ),
              message: 'An instruction engine must not import another engine (plan 192).',
            },
          ],
        },
      ],
    },
  })),
  {
    files: ['src/**/*.{ts,tsx}'],
    ignores: ['src/Crawler/engines/**'],
    rules: {
      'no-restricted-imports': [
        'error',
        {
          patterns: [
            {
              group: ['**/engines/rod', '**/engines/rod/*', '**/engines/katana', '**/engines/katana/*'],
              message: 'Reach an instruction engine through engines/registry (plan 192).',
            },
          ],
        },
      ],
    },
  },
  eslintConfigPrettier,
)
