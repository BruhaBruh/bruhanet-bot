import js from '@eslint/js'
import { defineConfig, globalIgnores } from 'eslint/config'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import globals from 'globals'
import tseslint from 'typescript-eslint'

export default defineConfig([
  globalIgnores(['dist']),
  js.configs.recommended,
  tseslint.configs.recommended,
  { languageOptions: { globals: globals.browser } },
  { files: ['scripts/**', '*.{js,ts}'], languageOptions: { globals: globals.node } },
  {
    files: ['src/**/*.tsx'],
    extends: [reactHooks.configs.flat.recommended, reactRefresh.configs.vite],
  },
  // Легаси: файлы переписываются целиком, правила снимаются точечно и уходят вместе с ними.
  {
    // до замены в Task 6 (Broadcasts, Composer, PhotoPicker), Task 7 (Promos), Task 10 (Texts)
    files: [
      'src/pages/Broadcasts.tsx',
      'src/compose/Composer.tsx',
      'src/compose/PhotoPicker.tsx',
      'src/pages/Promos.tsx',
      'src/pages/Texts.tsx',
    ],
    rules: { 'react-hooks/refs': 'off', 'react-hooks/set-state-in-effect': 'off' },
  },
  {
    // до замены в Task 6 (Composer.test), Task 7 (promos.test), Task 10 (texts.test)
    files: ['src/compose/Composer.test.tsx', 'src/pages/promos.test.tsx', 'src/pages/texts.test.tsx'],
    rules: { '@typescript-eslint/no-explicit-any': 'off', '@typescript-eslint/no-unused-vars': 'off' },
  },
])
