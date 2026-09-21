import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';

import AlertsView from './AlertsView';
import { DialogContext, type DialogApi } from './ui/dialog-context';
import { SessionContext, type SessionState } from './ui/session-context';
import { SiteScopeContext, type SiteScopeState } from './ui/site-scope-context';

type LeitorDeArquivo = { readFileSync(caminho: URL, codificacao: 'utf8'): string };

const fs = (
  globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): LeitorDeArquivo } }
).process.getBuiltinModule('node:fs');

const ler = (caminho: string) => fs.readFileSync(new URL(caminho, import.meta.url), 'utf8');

const FIXTURE = '../__fixtures__/alerts.saida-de-backend-internal-api-alerts_handler.json';

const fixture = JSON.parse(ler(FIXTURE)) as Record<string, unknown>[];
const camposDaFixture = Object.keys(fixture[0]).sort();

const tagsJsonDoStruct = (fonte: string, nome: string): string[] => {
  const corpo = new RegExp(`type ${nome} struct \\{([\\s\\S]*?)\\n\\}`).exec(fonte);
  if (!corpo) throw new Error(`struct ${nome} não encontrado`);
  return [...corpo[1].matchAll(/json:"([a-z_]+)[",]/g)].map((m) => m[1]);
};

const camposDaInterface = (fonte: string, nome: string): string[] => {
  const corpo = new RegExp(`export interface ${nome} \\{([\\s\\S]*?)\\n\\}`).exec(fonte);
  if (!corpo) throw new Error(`interface ${nome} não encontrada`);
  return [...corpo[1].matchAll(/^\s+([a-z_]+)\??:/gm)].map((m) => m[1]);
};

const api = vi.hoisted(() => ({ alerts: vi.fn(), ackAlert: vi.fn(), resolveAlert: vi.fn() }));

vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api,
}));

const dialogo: DialogApi = { confirm: vi.fn(), prompt: vi.fn(), notify: vi.fn() };

const admin: SessionState = {
  username: 'p',
  role: 'admin',
  accesses: [{ site_id: null, role: 'admin' }],
  isToken: false,
  logout: vi.fn(),
};

const escopo: SiteScopeState = {
  siteId: 'all',
  numericSiteId: null,
  setSiteId: vi.fn(),
  sites: [],
  siteName: () => '—',
  reloadSites: vi.fn(),
};

const renderizar = () =>
  render(
    <SessionContext.Provider value={admin}>
      <SiteScopeContext.Provider value={escopo}>
        <DialogContext.Provider value={dialogo}>
          <AlertsView />
        </DialogContext.Provider>
      </SiteScopeContext.Provider>
    </SessionContext.Provider>,
  );

const vigiar = (linha: Record<string, unknown>, lidos: Set<string>) =>
  new Proxy(linha, {
    get(alvo, campo, receptor) {
      if (typeof campo === 'string') lidos.add(campo);
      return Reflect.get(alvo, campo, receptor);
    },
  });

beforeEach(() => {
  vi.clearAllMocks();
});

describe('contrato de GET /api/alerts', () => {
  it('a fixture tem exatamente os campos que o backend serializa', () => {
    const doBackend = [
      ...tagsJsonDoStruct(ler('../../../backend/internal/database/schema.go'), 'Alert'),
      ...tagsJsonDoStruct(ler('../../../backend/internal/api/alerts_handler.go'), 'alertaComOrigem'),
    ].sort();
    expect(camposDaFixture).toEqual(doBackend);
  });

  it('AlertItem declara exatamente os campos da fixture', () => {
    expect(camposDaInterface(ler('../lib/api.ts'), 'AlertItem').sort()).toEqual(camposDaFixture);
  });

  it('a tela não lê campo que a resposta real não traz', async () => {
    const lidos = new Set<string>();
    api.alerts.mockResolvedValue(fixture.map((linha) => vigiar(linha, lidos)));
    renderizar();
    await screen.findByText(String(fixture[0].text));
    const fora = [...lidos].filter((campo) => !camposDaFixture.includes(campo));
    expect(fora).toEqual([]);
  });

  it('renotify_count já é o número de renotificações', async () => {
    api.alerts.mockResolvedValue(fixture);
    renderizar();
    const item = (await screen.findByText(String(fixture[0].text))).closest('li');
    expect(within(item as HTMLElement).getByText(/renotificado 3 vezes, última/)).toBeTruthy();
  });

  it('entrega dispensada tem rótulo próprio e cor neutra', async () => {
    api.alerts.mockResolvedValue(fixture);
    renderizar();
    const rotulo = await screen.findByText('Dispensado');
    expect(rotulo.className).toContain('text-text-mut');
    expect(rotulo.className).not.toMatch(/text-ok|text-crit|text-warn/);
  });

  it('aviso de recuperação chega resolvido e a tela não filtra por sufixo de chave', () => {
    expect(fixture[1].status).toBe('resolved');
    expect(ler('./AlertsView.tsx')).not.toContain(':recuperacao');
  });
});
