import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { fileURLToPath } from 'node:url'

export default defineConfig({
  root: fileURLToPath(new URL('./styleguide', import.meta.url)),
  publicDir: fileURLToPath(new URL('./public', import.meta.url)),
  plugins: [react(), tailwindcss()],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  build: { outDir: fileURLToPath(new URL('./dist-styleguide', import.meta.url)), emptyOutDir: true, rolldownOptions: { input: { guide: fileURLToPath(new URL('./styleguide/index.html', import.meta.url)), samples: fileURLToPath(new URL('./styleguide/samples.html', import.meta.url)) } } },
  preview: { host: '127.0.0.1', port: 8082, strictPort: true },
})
