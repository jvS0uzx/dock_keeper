import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import DevicesView from './DevicesView';
import { SessionContext, type SessionState } from './ui/session-context';
import { SiteScopeContext, type SiteScopeState } from './ui/site-scope-context';
import { DialogContext, type DialogApi } from './ui/dialog-context';
import type { Role, SiteAccess } from '../lib/session';
import type { Site } from '../lib/api';

const UNIDADES: Site[] = [
  { id: 1, name: 'Matriz', code: 'matriz' } as Site,
  { id: 2, name: 'Filial Norte', code: 'norte' } as Site,
];

const cincoMinutosAtras = () => new Date(Date.now() - 5 * 60 * 1000).toISOString();

const dispositivoAtivo = () => ({
  device_id: 'dev-a',
  site_id: 1,
  kind: 'agent',
  machine_id: 'm-1',
  hostname: 'pc-financeiro-01',
  created_at: '2026-09-01T12:00:00Z',
  last_seen_at: cincoMinutosAtras(),
  revoked_at: null,
});

const dispositivoRevogado = {
  device_id: 'dev-c',
  site_id: 2,
  kind: 'collector',
  machine_id: 'm-2',
  hostname: 'coletor-norte',
  created_at: '2026-08-20T12:00:00Z',
  last_seen_at: null,
  revoked_at: '2026-09-10T10:00:00Z',
};

interface Chamada {
  method: string;
  url: string;
  body: unknown;
}

let chamadas: Chamada[];
let revogado: boolean;
let erroDaLista: string | null;

const responder = (status: number, corpo: unknown) =>
  new Response(JSON.stringify(corpo), { status, headers: { 'Content-Type': 'application/json' } });

beforeEach(() => {
  chamadas = [];
  revogado = false;
  erroDaLista = null;
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? 'GET';
      chamadas.push({ method, url, body: init?.body ? JSON.parse(String(init.body)) : undefined });

      if (url.endsWith('/api/devices') && method === 'GET') {
        if (erroDaLista) return responder(500, { error: erroDaLista });
        const ativo = dispositivoAtivo();
        return responder(200, [
          revogado ? { ...ativo, revoked_at: new Date().toISOString() } : ativo,
          dispositivoRevogado,
        ]);
      }
      if (url.includes('/api/devices?device_id=') && method === 'DELETE') {
        revogado = true;
        return responder(200, { status: 'revogado' });
      }
      if (url.endsWith('/api/enroll/tokens') && method === 'POST') {
        const corpo = JSON.parse(String(init?.body));
        return responder(201, {
          enrollment_token: 'convite-secreto-123',
          site_id: corpo.site_id,
          kind: corpo.kind,
          expires_at: '2026-09-19T15:30:00Z',
        });
      }
      return responder(404, { error: 'rota inesperada' });
    }),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

const renderComPapel = (role: Role, accesses: SiteAccess[], dialogo?: Partial<DialogApi>) => {
  const sessao: SessionState = {
    username: 'pessoa-de-teste',
    role,
    accesses,
    isToken: false,
    logout: vi.fn(),
  };
  const escopo: SiteScopeState = {
    siteId: 'all',
    numericSiteId: null,
    setSiteId: vi.fn(),
    sites: UNIDADES,
    siteName: (id) => UNIDADES.find((s) => s.id === id)?.name ?? '—',
    reloadSites: vi.fn(),
  };
  const dialogoFalso: DialogApi = {
    confirm: vi.fn(async () => true),
    prompt: vi.fn(async () => null),
    notify: vi.fn(),
    ...dialogo,
  };

  render(
    <SessionContext.Provider value={sessao}>
      <SiteScopeContext.Provider value={escopo}>
        <DialogContext.Provider value={dialogoFalso}>
          <DevicesView />
        </DialogContext.Provider>
      </SiteScopeContext.Provider>
    </SessionContext.Provider>,
  );
  return dialogoFalso;
};

const ADMIN_GLOBAL: SiteAccess[] = [{ site_id: null, role: 'admin' }];

const linhaDe = async (hostname: string) => {
  const celula = await screen.findByText(hostname);
  const linha = celula.closest('tr');
  if (!linha) throw new Error(`linha de ${hostname} não encontrada`);
  return within(linha);
};

describe('DevicesView', () => {
  it('lista tipo, unidade, hostname, última atividade e estado', async () => {
    renderComPapel('admin', ADMIN_GLOBAL);

    const ativo = await linhaDe('pc-financeiro-01');
    expect(ativo.getByText('Agente de estação')).toBeTruthy();
    expect(ativo.getByText('Matriz')).toBeTruthy();
    expect(ativo.getByText('há 5min')).toBeTruthy();
    expect(ativo.getByText('Ativo')).toBeTruthy();
    expect(ativo.getByRole('button', { name: /revogar/i })).toBeTruthy();

    const antigo = await linhaDe('coletor-norte');
    expect(antigo.getByText('Coletor de rede')).toBeTruthy();
    expect(antigo.getByText('Filial Norte')).toBeTruthy();
    expect(antigo.getByText('nunca')).toBeTruthy();
    expect(antigo.getByText('Revogado')).toBeTruthy();
    expect(antigo.queryByRole('button', { name: /revogar/i })).toBeNull();
  });

  it('emite convite com tipo e unidade e mostra o token uma vez', async () => {
    const user = userEvent.setup();
    const escrever = vi.fn(async () => {});
    Object.defineProperty(navigator, 'clipboard', { value: { writeText: escrever }, configurable: true });
    renderComPapel('admin', ADMIN_GLOBAL);
    await screen.findByText('pc-financeiro-01');

    await user.click(screen.getByRole('button', { name: 'Coletor de rede' }));
    await user.click(screen.getByRole('combobox', { name: 'Unidade do convite' }));
    await user.click(screen.getByRole('option', { name: 'Filial Norte' }));
    await user.click(screen.getByRole('button', { name: /emitir convite/i }));

    await screen.findByText('convite-secreto-123');
    const emissao = chamadas.find((c) => c.method === 'POST');
    expect(emissao?.url).toMatch(/\/api\/enroll\/tokens$/);
    expect(emissao?.body).toEqual({ kind: 'collector', site_id: 2 });
    expect(screen.getByText(/COLLECTOR_ENROLL_TOKEN/)).toBeTruthy();
    expect(screen.getByText(/Vale até/)).toBeTruthy();

    await user.click(screen.getByRole('button', { name: /copiar/i }));
    expect(escrever).toHaveBeenCalledWith('convite-secreto-123');
    expect(screen.getByRole('button', { name: /copiado/i })).toBeTruthy();

    await user.click(screen.getByRole('button', { name: /já guardei/i }));
    expect(screen.queryByText('convite-secreto-123')).toBeNull();
  });

  it('revoga com confirmação e recarrega a lista', async () => {
    const user = userEvent.setup();
    const dialogo = renderComPapel('admin', ADMIN_GLOBAL);

    const ativo = await linhaDe('pc-financeiro-01');
    await user.click(ativo.getByRole('button', { name: /revogar/i }));

    expect(dialogo.confirm).toHaveBeenCalledWith(expect.objectContaining({ danger: true }));
    const revogacao = chamadas.find((c) => c.method === 'DELETE');
    expect(revogacao?.url).toMatch(/\/api\/devices\?device_id=dev-a$/);

    await waitFor(async () => {
      const linha = await linhaDe('pc-financeiro-01');
      expect(linha.getByText('Revogado')).toBeTruthy();
    });
  });

  it('não revoga quando a confirmação é negada', async () => {
    const user = userEvent.setup();
    renderComPapel('admin', ADMIN_GLOBAL, { confirm: vi.fn(async () => false) });

    const ativo = await linhaDe('pc-financeiro-01');
    await user.click(ativo.getByRole('button', { name: /revogar/i }));

    expect(chamadas.some((c) => c.method === 'DELETE')).toBe(false);
  });

  it('não carrega dispositivos para quem não é administrador global', async () => {
    renderComPapel('admin', [{ site_id: 1, role: 'admin' }]);

    expect(await screen.findByText(/não permite esta tela/)).toBeTruthy();
    expect(chamadas.some((c) => c.url.includes('/api/devices'))).toBe(false);
  });
});

describe('DevicesView — falha não vira vazio', () => {
  it('erro ao listar mostra a mensagem no lugar de "nenhum dispositivo"', async () => {
    erroDaLista = 'falha ao listar dispositivos';
    renderComPapel('admin', ADMIN_GLOBAL);

    expect(await screen.findByText('falha ao listar dispositivos')).toBeTruthy();
    expect(screen.queryByText(/Nenhum dispositivo cadastrado/)).toBeNull();
  });
});
