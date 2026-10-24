import js from '@eslint/js'
import tseslint from 'typescript-eslint'
import hooks from 'eslint-plugin-react-hooks'
import globals from 'globals'

export default tseslint.config(
  { ignores: ['dist/**', '.pattern-dist/**', 'test-results-patterns/**', 'dist-admin/**', 'dist-styleguide/**', 'styleguide-test-results/**', 'node_modules/**', 'src/api/schema.gen.ts', 'playwright-report/**', 'test-results/**', 'test-results-communications/**'] },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  { languageOptions: { globals: { ...globals.browser, ...globals.node } } },
  { files: ['src/**/*.{ts,tsx}'], plugins: { 'react-hooks': hooks }, rules: hooks.configs.recommended.rules },
)
