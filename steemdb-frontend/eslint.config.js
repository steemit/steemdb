import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'
import { defineConfig, globalIgnores } from 'eslint/config'

export default defineConfig([
  globalIgnores(['dist']),
  {
    files: ['**/*.{ts,tsx}'],
    extends: [
      js.configs.recommended,
      tseslint.configs.recommended,
      reactHooks.configs.flat.recommended,
      reactRefresh.configs.vite,
    ],
    languageOptions: {
      ecmaVersion: 2020,
      globals: globals.browser,
    },
    rules: {
      // XSS fence: every string the app displays (post bodies, titles,
      // witness URLs, account profiles, operation payloads) comes from the
      // chain, i.e. from whoever wrote it. React escapes text children, and
      // escaped text is the ONLY rendering mode in this app — the post body
      // is shown as literal text with whitespace-pre-wrap, never as HTML or
      // markdown. Raw-HTML sinks bypass that escape entirely, so the ones
      // that can reach the DOM are banned outright: a markdown/embed
      // renderer needs an allow-list sanitizer (e.g. DOMPurify) reviewed in
      // the same PR. See docs/AI-driver/04-web-service.md ("Output escaping"
      // contract).
      'no-restricted-syntax': [
        'error',
        {
          selector: 'JSXAttribute[name.name="dangerouslySetInnerHTML"]',
          message:
            'Chain data must render as React text. Adding raw HTML requires an allow-list sanitizer reviewed in the same PR (docs/AI-driver/04-web-service.md, "Output escaping").',
        },
        {
          // el.innerHTML = … / el.outerHTML = …, including el['innerHTML'] = …
          selector:
            'AssignmentExpression:matches([left.property.name=/^(inner|outer)HTML$/], [left.property.value=/^(inner|outer)HTML$/])',
          message:
            'Direct innerHTML/outerHTML assignment is an XSS sink for chain data. Render through JSX instead (docs/AI-driver/04-web-service.md, "Output escaping").',
        },
        {
          // el.insertAdjacentHTML('beforeend', …)
          selector:
            'CallExpression:matches([callee.property.name="insertAdjacentHTML"], [callee.property.value="insertAdjacentHTML"])',
          message:
            'insertAdjacentHTML is an XSS sink for chain data. Render through JSX instead (docs/AI-driver/04-web-service.md, "Output escaping").',
        },
      ],
    },
  },
])
