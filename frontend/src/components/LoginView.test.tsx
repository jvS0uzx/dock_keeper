import { describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import LoginView from './LoginView';
import { loadSession } from '../lib/session';
import type { SessionInfo } from '../lib/session';

vi.mock('../lib/api', () => ({
  api: { login: vi.fn() },
}));

import { api } from '../lib/api';

const sessaoValida: SessionInfo = {
  token: 'token-de-teste',
  user_id: 1,
  username: 'admin',
  role: 'admin',
  expires_at: new Date(Date.now() + 12 * 60 * 60 * 1000).toISOString(),
  accesses: [{ site_id: null, role: 'admin' }],
};

const preencherEEnviar = async (usuario: string, senha: string) => {
  const user = userEvent.setup();
  await user.type(screen.getByLabelText('Usuário'), usuario);
  await user.type(screen.getByLabelText('Senha'), senha);
  await user.click(screen.getByRole('button', { name: /entrar/i }));
  return user;
};

describe('LoginView', () => {
  it('credencial aceita persiste a sessão e avisa o App', async () => {
    vi.mocked(api.login).mockResolvedValue(sessaoValida);
    const onLogin = vi.fn();

    render(<LoginView onLogin={onLogin} />);
    await preencherEEnviar('admin', 'senha-correta-123');

    await waitFor(() => expect(onLogin).toHaveBeenCalledWith(sessaoValida));
    expect(api.login).toHaveBeenCalledWith('admin', 'senha-correta-123');
    expect(loadSession()?.token).toBe('token-de-teste');
  });

  it('credencial recusada mostra a mensagem que a API devolveu', async () => {
    vi.mocked(api.login).mockRejectedValue(new Error('{"error":"usuário ou senha inválidos"}'));
    const onLogin = vi.fn();

    render(<LoginView onLogin={onLogin} />);
    await preencherEEnviar('admin', 'senha-errada');

    const aviso = await screen.findByRole('alert');
    expect(aviso.textContent).toBe('usuário ou senha inválidos');
    expect(onLogin).not.toHaveBeenCalled();
    expect(loadSession()).toBeNull();
  });

  it('backend fora do ar produz mensagem sobre a API, não sobre a senha', async () => {
    vi.mocked(api.login).mockRejectedValue(new Error('Failed to fetch'));

    render(<LoginView onLogin={vi.fn()} />);
    await preencherEEnviar('admin', 'qualquer-senha');

    const aviso = await screen.findByRole('alert');
    expect(aviso.textContent).toContain('API');
  });

  it('o botão fica desabilitado enquanto usuário ou senha estiverem vazios', async () => {
    const user = userEvent.setup();
    render(<LoginView onLogin={vi.fn()} />);

    const botao = screen.getByRole('button', { name: /entrar/i }) as HTMLButtonElement;
    expect(botao.disabled).toBe(true);

    await user.type(screen.getByLabelText('Usuário'), 'admin');
    expect(botao.disabled).toBe(true);

    await user.type(screen.getByLabelText('Senha'), 'senha');
    expect(botao.disabled).toBe(false);
  });

  it('exibe o aviso recebido do App', () => {
    render(<LoginView onLogin={vi.fn()} notice="Sessão expirada. Entre novamente." />);
    expect(screen.getByText('Sessão expirada. Entre novamente.')).toBeTruthy();
  });
});
