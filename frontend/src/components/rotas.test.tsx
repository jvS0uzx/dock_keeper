import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import App from '../App';
import { saveSession, clearSession, type SessionInfo } from '../lib/session';

vi.mock('../config', () => ({
  API_TOKEN: 'token-de-teste',
  apiBase: () => '',
  loadRuntimeConfig: () => Promise.resolve(),
}));

vi.mock('./Dashboard', () => ({ default: () => <div>tela do dashboard</div> }));
vi.mock('./ServersView', () => ({ default: () => <div>tela de servidores</div> }));
vi.mock('./AlertsView', () => ({ default: () => <div>tela de alertas</div> }));
vi.mock('./UsersView', () => ({ default: () => <div>tela de usuarios</div> }));
vi.mock('./StationsView', () => ({ default: () => <div>tela de estacoes</div> }));
vi.mock('./MachineDetailView', () => ({ default: () => <div>detalhe da maquina</div> }));

const api = vi.hoisted(() => ({
  sites: vi.fn(),
  alertsSummary: vi.fn(),
  login: vi.fn(),
  logout: vi.fn(),
  readiness: vi.fn(),
}));

vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api,
}));

const sessaoAdmin: SessionInfo = {
  token: 'tok',
  user_id: 1,
  username: 'joaosouza@example.com',
  role: 'admin',
  expires_at: new Date(Date.now() + 3600_000).toISOString(),
  accesses: [{ site_id: null, role: 'admin' }],
};

const sessaoViewer: SessionInfo = { ...sessaoAdmin, role: 'viewer', accesses: [{ site_id: null, role: 'viewer' }] };

const irPara = (caminho: string) => window.history.replaceState({}, '', caminho);

beforeEach(() => {
  localStorage.clear();
  irPara('/dashboard');
  api.sites.mockReset().mockResolvedValue([]);
  api.alertsSummary.mockReset().mockResolvedValue({ open: 0, acked: 0, falhou: 0 });
  api.logout.mockReset().mockResolvedValue(undefined);
  api.readiness.mockReset().mockResolvedValue({ status: 'ok' });
  api.login.mockReset().mockResolvedValue(sessaoAdmin);
});

describe('rotas do painel', () => {
  it('clicar na barra lateral muda a URL', async () => {
    saveSession(sessaoAdmin);
    render(<App />);

    await userEvent.click(await screen.findByText('Servidores'));

    expect(window.location.pathname).toBe('/servidores');
    expect(await screen.findByText('tela de servidores')).toBeTruthy();
  });

  it('recarregar em um caminho abre a tela daquele caminho', async () => {
    saveSession(sessaoAdmin);
    irPara('/alertas');
    render(<App />);

    expect(await screen.findByText('tela de alertas')).toBeTruthy();
  });

  it('o caminho decide o painel, sem a aba discordar da URL', async () => {
    saveSession(sessaoAdmin);
    irPara('/estacoes');
    render(<App />);

    expect(await screen.findByText('tela de estacoes')).toBeTruthy();
    expect(screen.getByText('Unidades, estações e inventário')).toBeTruthy();
  });

  it('voltar e avançar do navegador funcionam', async () => {
    saveSession(sessaoAdmin);
    render(<App />);
    await screen.findByText('tela do dashboard');

    await userEvent.click(screen.getByText('Servidores'));
    await screen.findByText('tela de servidores');

    window.history.back();
    await waitFor(() => expect(window.location.pathname).toBe('/dashboard'));
    expect(await screen.findByText('tela do dashboard')).toBeTruthy();

    window.history.forward();
    await waitFor(() => expect(window.location.pathname).toBe('/servidores'));
    expect(await screen.findByText('tela de servidores')).toBeTruthy();
  });

  it('caminho desconhecido cai na tela inicial, sem tela branca', async () => {
    saveSession(sessaoAdmin);
    irPara('/nao-existe');
    render(<App />);

    expect(await screen.findByText('tela do dashboard')).toBeTruthy();
    await waitFor(() => expect(window.location.pathname).toBe('/dashboard'));
  });

  it('tela sem permissão cai na inicial', async () => {
    saveSession(sessaoViewer);
    irPara('/usuarios');
    render(<App />);

    expect(await screen.findByText('tela do dashboard')).toBeTruthy();
    expect(screen.queryByText('tela de usuarios')).toBeNull();
  });

  it('sem sessão vai para /login e volta ao destino depois de entrar', async () => {
    clearSession();
    irPara('/paineis');
    render(<App />);

    await waitFor(() => expect(window.location.pathname).toBe('/login'));

    await userEvent.type(screen.getByLabelText('Usuário ou e-mail'), 'joao');
    await userEvent.type(screen.getByLabelText('Senha'), 'senha-de-teste-1234');
    await userEvent.click(screen.getByRole('button', { name: 'Entrar' }));

    await waitFor(() => expect(window.location.pathname).toBe('/paineis'));
  });

  it('detalhe de máquina tem caminho próprio', async () => {
    saveSession(sessaoAdmin);
    irPara('/maquinas/srv-1');
    render(<App />);

    expect(await screen.findByText('detalhe da maquina')).toBeTruthy();
  });
});

describe('modo token', () => {
  it('não entra sozinho: mostra o login com o botão de token', async () => {
    clearSession();
    irPara('/dashboard');
    render(<App />);

    expect(await screen.findByLabelText('Usuário ou e-mail')).toBeTruthy();
    expect(screen.queryByText('tela do dashboard')).toBeNull();
    expect(screen.getByRole('button', { name: /token \(dev\)/i })).toBeTruthy();
  });

  it('entra só depois do clique, e o rodapé avisa que é máquina', async () => {
    clearSession();
    irPara('/dashboard');
    render(<App />);

    await userEvent.click(await screen.findByRole('button', { name: /token \(dev\)/i }));

    expect(await screen.findByText('tela do dashboard')).toBeTruthy();
    expect(screen.getByText('api-token (dev)')).toBeTruthy();
  });
});
