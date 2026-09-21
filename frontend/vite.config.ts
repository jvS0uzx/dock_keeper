import { readFileSync } from 'node:fs'
import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

const pacote = JSON.parse(
  readFileSync(new URL('./package.json', import.meta.url), 'utf8'),
) as { version: string }

export default defineConfig({
  plugins: [react()],
  define: {
    __APP_VERSION__: JSON.stringify(pacote.version),
  },
  test: {
    environment: 'jsdom',
    restoreMocks: true,
    setupFiles: ['./src/test-setup.ts'],
  },
})
