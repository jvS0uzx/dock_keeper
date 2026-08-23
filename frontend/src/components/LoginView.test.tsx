import { describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import LoginView from './LoginView';
import { loadSession } from '../lib/session';
import type { SessionInfo } from '../lib/session';

// api é mockado inteiro: a tela de login é a única porta de entrada do painel e
// precisa ser testável sem backend no ar.
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
  // Credencial aceita precisa fazer duas coisas, e as duas importam: avisar o
  // App (senão a tela de login continua na frente) e persistir a sessão (senão
  // recarregar a página derruba o usuário de volta para cá).
  it('credencial aceita persiste a sessão e avisa o App', async () => {
    vi.mocked(api.login).mockResolvedValue(sessaoValida);
    const onLogin = vi.fn();

    render(<LoginView onLogin={onLogin} />);
    await preencherEEnviar('admin', 'senha-correta-123');

    await waitFor(() => expect(onLogin).toHaveBeenCalledWith(sessaoValida));
    expect(api.login).toHaveBeenCalledWith('admin', 'senha-correta-123');
    expect(loadSession()?.token).toBe('token-de-teste');
  });

  // O backend responde {"error": "..."} e a tela precisa mostrar ESSA mensagem.
  // Engolir o motivo real deixa o usuário sem saber se errou a senha ou se a
  // conta está desativada, e o suporte sem saber o que perguntar.
  it('credencial recusada mostra a mensagem que a API devolveu', async () => {
    vi.mocked(api.login).mockRejectedValue(new Error('{"error":"usuário ou senha inválidos"}'));
    const onLogin = vi.fn();

    render(<LoginView onLogin={onLogin} />);
    await preencherEEnviar('admin', 'senha-errada');

    const aviso = await screen.findByRole('alert');
    expect(aviso.textContent).toBe('usuário ou senha inválidos');
    expect(onLogin).not.toHaveBeenCalled();
    // Sessão recusada não pode ficar guardada: o App leria o storage no próximo
    // carregamento e entraria com uma credencial que o backend já negou.
    expect(loadSession()).toBeNull();
  });

  // Backend fora do ar produz "Failed to fetch", que não é JSON. Repassar isso
  // cru manda o usuário procurar erro de senha onde o problema é rede.
  it('backend fora do ar produz mensagem sobre a API, não sobre a senha', async () => {
    vi.mocked(api.login).mockRejectedValue(new Error('Failed to fetch'));

    render(<LoginView onLogin={vi.fn()} />);
    await preencherEEnviar('admin', 'qualquer-senha');

    const aviso = await screen.findByRole('alert');
    expect(aviso.textContent).toContain('API');
  });

  // O botão fica travado enquanto falta campo. Sem isso o formulário dispara uma
  // requisição vazia, que o backend recusa — e cada tentativa dessas gasta um
  // bcrypt e conta no limite de tentativa por IP.
  it('o botão fica desabilitado enquanto usuário ou senha estiverem vazios', async () => {
    const user = userEvent.setup();
    render(<LoginView onLogin={vi.fn()} />);

    // A propriedade do DOM, e não um matcher de jest-dom: uma dependência a
    // menos para uma asserção que não fica mais legível com ela.
    const botao = screen.getByRole('button', { name: /entrar/i }) as HTMLButtonElement;
    expect(botao.disabled).toBe(true);

    await user.type(screen.getByLabelText('Usuário'), 'admin');
    expect(botao.disabled).toBe(true);

    await user.type(screen.getByLabelText('Senha'), 'senha');
    expect(botao.disabled).toBe(false);
  });

  // O aviso de sessão expirada vem do App quando o backend responde 401 numa
  // sessão que existia. Se ele não aparecer, o usuário é jogado no login sem
  // explicação e acha que perdeu a senha.
  it('exibe o aviso recebido do App', () => {
    render(<LoginView onLogin={vi.fn()} notice="Sessão expirada. Entre novamente." />);
    expect(screen.getByText('Sessão expirada. Entre novamente.')).toBeTruthy();
  });
});
