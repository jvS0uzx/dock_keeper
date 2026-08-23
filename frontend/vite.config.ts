// defineConfig vem de 'vitest/config', não de 'vite': o defineConfig do vite não
// conhece a chave `test`, e usá-lo aqui quebra o `tsc -b` — ou seja, quebra o
// `npm run build`, não só os testes.
import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  test: {
    // jsdom só para os testes de componente. Os testes de src/lib/ são funções
    // puras e rodavam sem ambiente de DOM; o custo de subir o jsdom para eles
    // é irrelevante perto de ter que separar duas configurações.
    environment: 'jsdom',
    // Limpa o DOM entre os testes. Sem isso, o componente do teste anterior
    // continua montado e uma busca por texto encontra o elemento errado — e a
    // falha aparece no teste seguinte, não no que a causou.
    restoreMocks: true,
    setupFiles: ['./src/test-setup.ts'],
  },
})
