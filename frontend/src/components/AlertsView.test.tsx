import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import AlertsView from './AlertsView';
import Sidebar from './Sidebar';
import { DialogContext, type DialogApi } from './ui/dialog-context';
import { SessionContext, type SessionState } from './ui/session-context';
import { SiteScopeContext, type SiteScopeState } from './ui/site-scope-context';

const alerta = {
  id: 1,
  key: 'cpu:srv-1',
  severity: 'critical',
  text: 'CPU acima de 90% em vps-loja',
  status: 'open',
  server_id: 'srv-1',
  site_id: 3,
  rule_id: 7,
  created_at: '2026-09-18T12:00:00Z',
  acked_at: null,
  acked_by: null,
  resolved_at: null,
  delivery: 'falhou',
  attempts: 3,
  next_attempt_at: null,
  last_attempt_at: '2026-09-18T12:04:00Z',
  last_error: 'telegram fora do ar',
};

const entregue = {
  ...alerta,
  id: 2,
  key: 'ssl:api',
  severity: 'warning',
  text: 'Certificado de api.exemplo vence em 10 dias',
  delivery: 'enviado',
  attempts: 1,
  last_error: '',
};

const api = vi.hoisted(() => ({
  alerts: vi.fn(),
  alertsSummary: vi.fn(),
  ackAlert: vi.fn(),
  resolveAlert: vi.fn(),
}));

vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api,
}));

const dialogo: DialogApi = { confirm: vi.fn(), prompt: vi.fn(), notify: vi.fn() };

const sessao: SessionState = {
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
  sites: [{ id: 3, name: 'Filial Norte', code: 'norte' }] as SiteScopeState['sites'],
  siteName: () => 'Filial Norte',
  reloadSites: vi.fn(),
};

const renderizar = (tela: React.ReactElement) =>
  render(
    <SessionContext.Provider value={sessao}>
      <SiteScopeContext.Provider value={escopo}>
        <DialogContext.Provider value={dialogo}>{tela}</DialogContext.Provider>
      </SiteScopeContext.Provider>
    </SessionContext.Provider>,
  );

beforeEach(() => {
  api.alerts.mockReset().mockResolvedValue([alerta, entregue]);
  api.alertsSummary.mockReset().mockResolvedValue({ open: 2, acked: 1, falhou: 1 });
  api.ackAlert.mockReset().mockResolvedValue({ ...alerta, status: 'acked' });
  api.resolveAlert.mockReset().mockResolvedValue({ ...alerta, status: 'resolved' });
  (dialogo.confirm as ReturnType<typeof vi.fn>).mockReset().mockResolvedValue(true);
  (dialogo.notify as ReturnType<typeof vi.fn>).mockReset();
});

const linhaDe = async (texto: string) => {
  const celula = await screen.findByText(texto);
  const linha = celula.closest('tr');
  if (!linha) throw new Error(`linha de ${texto} não encontrada`);
  return within(linha);
};

describe('AlertsView', () => {
  it('mostra a entrega que falhou, com motivo e tentativas', async () => {
    renderizar(<AlertsView />);

    const linha = await linhaDe('CPU acima de 90% em vps-loja');
    expect(linha.getByText(/falhou/i)).toBeTruthy();
    expect(linha.getByText(/telegram fora do ar/i)).toBeTruthy();
    expect(linha.getByText(/3 tentativas/i)).toBeTruthy();

    const ok = await linhaDe('Certificado de api.exemplo vence em 10 dias');
    expect(ok.getByText(/enviado/i)).toBeTruthy();
  });

  it('filtra por estado e por severidade', async () => {
    const usuario = userEvent.setup();
    renderizar(<AlertsView />);
    await screen.findByText('CPU acima de 90% em vps-loja');

    await usuario.click(screen.getByLabelText('Estado'));
    await usuario.click(await screen.findByRole('option', { name: 'Resolvidos' }));
    await vi.waitFor(() =>
      expect(api.alerts).toHaveBeenCalledWith(expect.objectContaining({ status: 'resolved' }), expect.anything()),
    );

    await usuario.click(screen.getByLabelText('Severidade'));
    await usuario.click(await screen.findByRole('option', { name: 'Atenção' }));
    await vi.waitFor(() => {
      const linhas = screen.getAllByRole('row');
      expect(linhas.some((l) => /Certificado de api/.test(l.textContent ?? ''))).toBe(true);
      expect(linhas.some((l) => /CPU acima de 90/.test(l.textContent ?? ''))).toBe(false);
    });
  });

  it('reconhece com confirmação e recarrega', async () => {
    const usuario = userEvent.setup();
    renderizar(<AlertsView />);

    const linha = await linhaDe('CPU acima de 90% em vps-loja');
    await usuario.click(linha.getByRole('button', { name: /reconhecer/i }));

    expect(dialogo.confirm).toHaveBeenCalled();
    await vi.waitFor(() => expect(api.ackAlert).toHaveBeenCalledWith(1));
  });

  it('não reconhece quando a confirmação é negada', async () => {
    (dialogo.confirm as ReturnType<typeof vi.fn>).mockResolvedValue(false);
    const usuario = userEvent.setup();
    renderizar(<AlertsView />);

    const linha = await linhaDe('CPU acima de 90% em vps-loja');
    await usuario.click(linha.getByRole('button', { name: /reconhecer/i }));

    expect(api.ackAlert).not.toHaveBeenCalled();
  });

  it('mostra o erro do backend no lugar da lista vazia', async () => {
    api.alerts.mockRejectedValue(new Error(JSON.stringify({ error: 'painel fora do ar' })));
    renderizar(<AlertsView />);

    expect(await screen.findByText(/painel fora do ar/)).toBeTruthy();
    expect(screen.queryByText(/Nenhum alerta/)).toBeNull();
  });
});

describe('Sidebar — contador de alertas abertos', () => {
  it('mostra quantos alertas estão abertos no item do menu', async () => {
    render(
      <SessionContext.Provider value={sessao}>
        <SiteScopeContext.Provider value={escopo}>
          <Sidebar activeTab="dashboard" setActiveTab={vi.fn()} panel="dev" setPanel={vi.fn()} />
        </SiteScopeContext.Provider>
      </SessionContext.Provider>,
    );

    const item = (await screen.findByText('Alertas')).closest('button');
    expect(item).toBeTruthy();
    expect(within(item as HTMLElement).getByText('2')).toBeTruthy();
  });
});
