import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { fileURLToPath } from 'node:url'

// An isolated verification entry imports actual production patterns. Neither
// portal's build/router includes this harness, synthetic controller or baseline.
export default defineConfig({
  root: fileURLToPath(new URL('./e2e/pattern-harness', import.meta.url)),
  plugins: [react(), tailwindcss()],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  build: { outDir: fileURLToPath(new URL('./.pattern-dist', import.meta.url)), emptyOutDir: true },
})
