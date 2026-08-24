import { useState, type FormEvent } from 'react';
import { Activity, Loader2, LogIn } from 'lucide-react';
import { api } from '../lib/api';
import { saveSession, type SessionInfo } from '../lib/session';

interface LoginViewProps {
  onLogin: (session: SessionInfo) => void;
  /** Aviso exibido acima do formulário (ex: "Sessão expirada"). */
  notice?: string;
}

/**
 * Tela de entrada do painel. Só aparece quando não há sessão nem token de
 * ambiente — o backend é quem decide se a credencial vale.
 */
const LoginView = ({ onLogin, notice }: LoginViewProps) => {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [sending, setSending] = useState(false);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (sending) return;

    setError('');
    setSending(true);
    try {
      const session = await api.login(username.trim(), password);
      saveSession(session);
      onLogin(session);
    } catch (err) {
      // A API devolve {"error": "..."} — extrai a mensagem sem vazar detalhes.
      const raw = (err as Error).message;
      let message = 'Falha no login. Tente novamente.';
      try {
        message = (JSON.parse(raw) as { error?: string }).error ?? message;
      } catch {
        if (raw.toLowerCase().includes('failed to fetch')) {
          message = 'Não foi possível falar com a API. Verifique se o backend está no ar.';
        }
      }
      setError(message);
    } finally {
      setSending(false);
    }
  };

  return (
    <div className="flex h-screen w-screen items-center justify-center bg-ink-950 p-4 text-text">
      <div className="anim-rise w-full max-w-sm">
        <div className="mb-8 flex flex-col items-center">
          <div className="mb-4 flex h-12 w-12 items-center justify-center rounded-card border border-line bg-ink-800 shadow-panel">
            <Activity size={22} strokeWidth={1.75} className="text-accent" />
          </div>
          <h1 className="text-xl font-bold tracking-tight text-text-hi">
            Dock<span className="text-accent">Keeper</span>
          </h1>
          <p className="mt-1 text-xs text-text-mut">Monitoramento de infraestrutura</p>
        </div>

        <form onSubmit={handleSubmit} className="panel flex flex-col gap-4 p-6">
          {notice && (
            <p className="rounded-ctrl border border-warn/25 bg-warn/10 px-3 py-2 text-xs text-warn">
              {notice}
            </p>
          )}

          <div>
            <label htmlFor="login-username" className="eyebrow mb-1.5 block">
              Usuário
            </label>
            <input
              id="login-username"
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              autoComplete="username"
              autoFocus
              required
              className="input-base w-full"
            />
          </div>

          <div>
            <label htmlFor="login-password" className="eyebrow mb-1.5 block">
              Senha
            </label>
            <input
              id="login-password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="current-password"
              required
              className="input-base w-full"
            />
          </div>

          {error && (
            <p
              role="alert"
              className="rounded-ctrl border border-crit/25 bg-crit/10 px-3 py-2 text-xs text-crit"
            >
              {error}
            </p>
          )}

          <button
            type="submit"
            disabled={sending || !username.trim() || !password}
            className="btn btn-primary mt-2 w-full disabled:opacity-40"
          >
            {sending ? (
              <Loader2 size={14} strokeWidth={1.75} className="animate-spin" />
            ) : (
              <LogIn size={14} strokeWidth={1.75} />
            )}
            {sending ? 'Entrando...' : 'Entrar'}
          </button>
        </form>

        <p className="mt-6 text-center text-xs text-text-faint">
          Acesso restrito. Fale com o administrador para obter uma conta.
        </p>
      </div>
    </div>
  );
};

export default LoginView;
