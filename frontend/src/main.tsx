import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'
import { loadRuntimeConfig } from './config'

// A configuração precisa estar resolvida antes do primeiro render: um efeito
// inicial que dispare requisição usaria a URL antiga, e o sintoma seria uma
// falha de rede intermitente só na primeira carga da página.
loadRuntimeConfig().then(() => {
  createRoot(document.getElementById('root')!).render(
    <StrictMode>
      <App />
    </StrictMode>,
  )
})
