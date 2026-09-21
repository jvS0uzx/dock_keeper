import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import { semEspera } from '../test/usuario';

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

const semCanal = {
  ...alerta,
  id: 3,
  key: 'disco:vps-mail',
  severity: 'warning',
  text: 'Disco acima de 85% em vps-mail',
  delivery: 'sem_canal',
  attempts: 0,
  last_attempt_at: null,
  next_attempt_at: null,
  last_error: '',
};

const pendente = {
  ...alerta,
  id: 4,
  key: 'ram:vps-api',
  severity: 'info',
  text: 'RAM acima de 80% em vps-api',
  delivery: 'pendente',
  attempts: 1,
  next_attempt_at: '2026-09-18T12:10:00Z',
  last_error: '',
};

const entregaEstranha = {
  ...alerta,
  id: 5,
  key: 'swap:vps-web',
  severity: 'info',
  text: 'Swap acima de 50% em vps-web',
  delivery: 'expirado',
  attempts: 0,
  last_attempt_at: null,
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

const itemBruto = async (texto: string) => {
  const celula = await screen.findByText(texto);
  const item = celula.closest('li');
  if (!item) throw new Error(`item de ${texto} não encontrado`);
  return item;
};

const linhaDe = async (texto: string) => within(await itemBruto(texto));

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

  it('o motivo longo da falha fica num detalhe que abre, não solto na linha', async () => {
    renderizar(<AlertsView />);

    const item = await itemBruto('CPU acima de 90% em vps-loja');
    const detalhe = within(item).getByText(/detalhe da entrega/i).closest('details');
    expect(detalhe).toBeTruthy();
    expect(within(detalhe as HTMLElement).getByText(/telegram fora do ar/i)).toBeTruthy();
    expect(within(detalhe as HTMLElement).getByText(/3 tentativas/i)).toBeTruthy();
    expect((detalhe as HTMLDetailsElement).open).toBe(false);
  });

  it('o alerta entregue não carrega detalhe de entrega', async () => {
    renderizar(<AlertsView />);

    const item = await itemBruto('Certificado de api.exemplo vence em 10 dias');
    expect(within(item).queryByText(/detalhe da entrega/i)).toBeNull();
  });

  it('o texto do alerta tem largura de leitura limitada', async () => {
    renderizar(<AlertsView />);

    const texto = await screen.findByText('CPU acima de 90% em vps-loja');
    expect(texto.className).toMatch(/max-w-prose/);
  });

  it('Resolver pesa mais que Reconhecer e os dois não ficam colados', async () => {
    renderizar(<AlertsView />);

    const linha = await linhaDe('CPU acima de 90% em vps-loja');
    const reconhecer = linha.getByRole('button', { name: /reconhecer/i });
    const resolver = linha.getByRole('button', { name: /resolver/i });

    expect(resolver.className).toMatch(/btn-primary/);
    expect(reconhecer.className).toMatch(/btn-ghost/);
    expect(reconhecer.className).not.toMatch(/btn-primary/);

    const acoes = resolver.parentElement as HTMLElement;
    expect(acoes.contains(reconhecer)).toBe(true);
    expect(acoes.className).toMatch(/gap-3/);
  });

  it('filtra por estado e por severidade', async () => {
    const usuario = semEspera();
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
      const linhas = screen.getAllByRole('listitem');
      expect(linhas.some((l) => /Certificado de api/.test(l.textContent ?? ''))).toBe(true);
      expect(linhas.some((l) => /CPU acima de 90/.test(l.textContent ?? ''))).toBe(false);
    });
  });

  it('reconhece com confirmação e recarrega', async () => {
    const usuario = semEspera();
    renderizar(<AlertsView />);

    const linha = await linhaDe('CPU acima de 90% em vps-loja');
    await usuario.click(linha.getByRole('button', { name: /reconhecer/i }));

    expect(dialogo.confirm).toHaveBeenCalled();
    await vi.waitFor(() => expect(api.ackAlert).toHaveBeenCalledWith(1));
  });

  it('não reconhece quando a confirmação é negada', async () => {
    (dialogo.confirm as ReturnType<typeof vi.fn>).mockResolvedValue(false);
    const usuario = semEspera();
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

describe('AlertsView — entrega sem canal', () => {
  beforeEach(() => {
    api.alerts.mockResolvedValue([alerta, entregue, semCanal, pendente]);
  });

  it('mostra os quatro estados de entrega na mesma lista', async () => {
    renderizar(<AlertsView />);

    const falha = await linhaDe('CPU acima de 90% em vps-loja');
    const ok = await linhaDe('Certificado de api.exemplo vence em 10 dias');
    const sem = await linhaDe('Disco acima de 85% em vps-mail');
    const fila = await linhaDe('RAM acima de 80% em vps-api');

    expect(falha.getByText('Falhou')).toBeTruthy();
    expect(ok.getByText('Enviado')).toBeTruthy();
    expect(sem.getByText('Sem canal')).toBeTruthy();
    expect(fila.getByText('Pendente')).toBeTruthy();
  });

  it('dá cor própria ao sem canal, nem o verde de entregue nem o vermelho de falha', async () => {
    renderizar(<AlertsView />);

    const sem = await linhaDe('Disco acima de 85% em vps-mail');
    const rotulo = sem.getByText('Sem canal').closest('span');
    if (!(rotulo instanceof HTMLElement)) throw new Error('rótulo de entrega não encontrado');

    expect(rotulo.className).not.toMatch(/text-ok/);
    expect(rotulo.className).not.toMatch(/text-crit/);
    expect(rotulo.className).toMatch(/text-info/);
  });

  it('explica no detalhe o que acontece quando o canal for configurado', async () => {
    const usuario = semEspera();
    renderizar(<AlertsView />);

    const sem = await linhaDe('Disco acima de 85% em vps-mail');
    await usuario.click(sem.getByText('Detalhe da entrega'));

    const item = await itemBruto('Disco acima de 85% em vps-mail');
    expect(item.textContent).toMatch(/configurar/i);
    expect(item.textContent).toMatch(/24 h/);
    expect(item.textContent).toMatch(/fila/i);
  });

  it('não conta o sem canal como tentativa de entrega', async () => {
    const usuario = semEspera();
    renderizar(<AlertsView />);

    const sem = await linhaDe('Disco acima de 85% em vps-mail');
    await usuario.click(sem.getByText('Detalhe da entrega'));

    const item = await itemBruto('Disco acima de 85% em vps-mail');
    expect(item.textContent).not.toMatch(/0 tentativas/);
  });

  it('mantém o filtro de estado e o contador do menu alheios à entrega', async () => {
    renderizar(<AlertsView />);

    await screen.findByText('Disco acima de 85% em vps-mail');
    expect(api.alerts).toHaveBeenCalledWith({ status: 'open' }, expect.anything());
  });

  it('não quebra a tela com um valor de entrega desconhecido', async () => {
    api.alerts.mockResolvedValue([entregaEstranha]);
    renderizar(<AlertsView />);

    const linha = await linhaDe('Swap acima de 50% em vps-web');
    expect(linha.getByText('expirado')).toBeTruthy();
    expect(screen.queryByText(/Falha ao carregar/)).toBeNull();
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
