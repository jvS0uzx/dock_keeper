import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { semEspera } from '../test/usuario';

import LoginView from './LoginView';
import Sidebar from './Sidebar';
import UsersView from './UsersView';
import { DialogContext, type DialogApi } from './ui/dialog-context';
import { SessionContext, type SessionState } from './ui/session-context';
import { SiteScopeContext, type SiteScopeState } from './ui/site-scope-context';

const contas = [
  {
    id: 1,
    username: 'joaosouza@exemplo.com.br',
    nome: 'João Vitor Souza',
    email: 'joaosouza@exemplo.com.br',
    role: 'admin' as const,
    active: true,
    last_login: null,
    created_at: '',
    accesses: [],
  },
];

const api = vi.hoisted(() => ({
  users: vi.fn(),
  sites: vi.fn(),
  createUser: vi.fn(),
  login: vi.fn(),
}));

vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api,
}));

const dialogo: DialogApi = { confirm: vi.fn(), prompt: vi.fn(), notify: vi.fn() };

beforeEach(() => {
  api.users.mockReset().mockResolvedValue(contas);
  api.sites.mockReset().mockResolvedValue([]);
  api.createUser.mockReset().mockResolvedValue(contas[0]);
  api.login.mockReset();
});

const sessao = (extra: Partial<SessionState> = {}): SessionState => ({
  username: 'joaosouza@exemplo.com.br',
  role: 'admin',
  accesses: [{ site_id: null, role: 'admin' }],
  isToken: false,
  logout: vi.fn(),
  ...extra,
});

const escopo: SiteScopeState = {
  siteId: 'all',
  setSiteId: vi.fn(),
  numericSiteId: null,
  sites: [],
  siteName: () => '',
  reloadSites: vi.fn(),
  sitesError: null,
};

const preencher = async (usuario: ReturnType<typeof userEvent.setup>, campo: HTMLElement, texto: string) => {
  await usuario.click(campo);
  await usuario.paste(texto);
};

describe('conta com nome e e-mail', () => {
  it('cria a conta enviando nome e e-mail', async () => {
    render(
      <SessionContext.Provider value={sessao()}>
        <DialogContext.Provider value={dialogo}>
          <UsersView />
        </DialogContext.Provider>
      </SessionContext.Provider>,
    );

    await screen.findByText('João Vitor Souza');
    const usuario = semEspera();

    await preencher(usuario, screen.getByLabelText('Usuário'), 'maria.silva');
    await preencher(usuario, screen.getByLabelText('Nome'), 'Maria Silva');
    await preencher(usuario, screen.getByLabelText('E-mail'), 'maria@exemplo.com.br');
    await preencher(usuario, screen.getByLabelText('Senha'), 'senha-de-teste-1234');
    await usuario.click(screen.getByRole('button', { name: /criar conta/i }));

    expect(api.createUser).toHaveBeenCalledWith(
      expect.objectContaining({
        username: 'maria.silva',
        nome: 'Maria Silva',
        email: 'maria@exemplo.com.br',
      }),
    );
  });

  it('a lista mostra o nome da pessoa', async () => {
    render(
      <SessionContext.Provider value={sessao()}>
        <DialogContext.Provider value={dialogo}>
          <UsersView />
        </DialogContext.Provider>
      </SessionContext.Provider>,
    );

    expect(await screen.findByText('João Vitor Souza')).toBeTruthy();
  });

  it('a barra lateral mostra o nome quando existe, e o usuário quando não', () => {
    const { unmount } = render(
      <SessionContext.Provider value={sessao({ nome: 'João Vitor Souza' })}>
        <SiteScopeContext.Provider value={escopo}>
          <Sidebar activeTab="dashboard" setActiveTab={vi.fn()} panel="dev" setPanel={vi.fn()} />
        </SiteScopeContext.Provider>
      </SessionContext.Provider>,
    );
    expect(screen.getByText('João Vitor Souza')).toBeTruthy();
    unmount();

    render(
      <SessionContext.Provider value={sessao()}>
        <SiteScopeContext.Provider value={escopo}>
          <Sidebar activeTab="dashboard" setActiveTab={vi.fn()} panel="dev" setPanel={vi.fn()} />
        </SiteScopeContext.Provider>
      </SessionContext.Provider>,
    );
    expect(screen.getByText('joaosouza@exemplo.com.br')).toBeTruthy();
  });

  it('o login avisa que aceita usuário ou e-mail', () => {
    render(<LoginView onLogin={vi.fn()} />);

    expect(screen.getByLabelText(/usuário ou e-mail/i)).toBeTruthy();
  });
});
