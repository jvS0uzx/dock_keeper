/**
 * Base da API, resolvida em TEMPO DE EXECUÇÃO.
 *
 * O Vite embute `import.meta.env.*` no bundle, então usá-lo para a URL da API
 * exigiria um build por unidade instalada — cada filial com um endereço de
 * painel diferente teria que compilar o próprio frontend. Carregar de
 * `/config.json`, servido ao lado do `index.html`, faz o mesmo artefato servir a
 * todas: quem instala edita um arquivo de uma linha.
 *
 * A ordem de precedência resolve os três cenários reais:
 *   1. `/config.json` — instalação empacotada, e o que o nginx da imagem serve;
 *   2. `VITE_API_URL` — desenvolvimento, onde o backend está noutra porta;
 *   3. mesma origem — o caso do proxy reverso, em que `/api` é local.
 */
let apiUrl = import.meta.env.VITE_API_URL || '';

/**
 * loadRuntimeConfig precisa terminar ANTES do primeiro render: um componente
 * que dispare uma requisição no `useEffect` inicial usaria a URL antiga, e o
 * erro apareceria como uma falha de rede intermitente só na primeira carga.
 * Chamado de `main.tsx`, antes do `createRoot`.
 */
export async function loadRuntimeConfig(): Promise<void> {
  try {
    const resp = await fetch('/config.json', { cache: 'no-store' });
    if (!resp.ok) return;

    const cfg: unknown = await resp.json();
    if (cfg && typeof cfg === 'object' && 'apiUrl' in cfg) {
      const valor = (cfg as { apiUrl?: unknown }).apiUrl;
      if (typeof valor === 'string' && valor.trim() !== '') {
        apiUrl = valor.trim();
      }
    }
  } catch {
    // Ausência de /config.json é o caso normal em desenvolvimento, não erro:
    // cai no VITE_API_URL ou na mesma origem. Falhar aqui deixaria a tela em
    // branco por causa de um arquivo opcional.
  }
}

/**
 * Base da API para montar as URLs das requisições. String vazia significa mesma
 * origem, que é o que o `fetch` já faz com um caminho relativo.
 */
export function apiBase(): string {
  return apiUrl;
}

// Token de máquina do backend (API_TOKEN), aceito só em desenvolvimento.
//
// O Vite embute o valor no bundle, e o backend trata esse token como admin
// global — quem abrisse o DevTools de um build de produção sairia de lá com
// acesso total, por cima de qualquer papel. Fora do dev a constante é string
// vazia, o ramo vira código morto e o valor não chega ao bundle; sem ele a
// aplicação exige login, que é o único caminho com papel por unidade.
export const API_TOKEN: string = import.meta.env.DEV ? import.meta.env.VITE_API_TOKEN || '' : '';
