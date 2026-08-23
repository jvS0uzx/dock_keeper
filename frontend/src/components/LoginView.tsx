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
    <div className="h-screen w-screen flex items-center justify-center bg-[radial-gradient(ellipse_at_top,_var(--tw-gradient-stops))] from-[#1a1c23] via-[#050505] to-[#000000] text-white font-sans p-4">
      <div className="w-full max-w-sm">
        <div className="flex flex-col items-center mb-8">
          <div className="w-14 h-14 rounded-2xl bg-gradient-to-br from-[#10b981]/10 to-[#0c0c0e] border border-[#10b981]/40 flex items-center justify-center shadow-[0_0_30px_rgba(16,185,129,0.15)] mb-4">
            <Activity className="w-7 h-7 text-[#10b981]" strokeWidth={1.5} />
          </div>
          <h1 className="text-2xl font-bold tracking-widest uppercase drop-shadow-md">
            VD <span className="text-[#10b981]">Stats</span>
          </h1>
          <p className="text-[10px] text-[#737373] mt-1 tracking-widest uppercase">
            Monitoramento de infraestrutura
          </p>
        </div>

        <form onSubmit={handleSubmit} className="glass-panel rounded-xl p-6 flex flex-col gap-4">
          {notice && (
            <p className="text-xs text-amber-400 bg-amber-400/10 border border-amber-400/20 rounded-lg px-3 py-2">
              {notice}
            </p>
          )}

          <div>
            <label htmlFor="login-username" className="text-xs text-[#737373] block mb-1">
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
              className="w-full bg-black/40 border border-white/10 rounded-lg px-4 py-2 text-sm text-white focus:outline-none focus:border-[#10b981] transition-colors"
            />
          </div>

          <div>
            <label htmlFor="login-password" className="text-xs text-[#737373] block mb-1">
              Senha
            </label>
            <input
              id="login-password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="current-password"
              required
              className="w-full bg-black/40 border border-white/10 rounded-lg px-4 py-2 text-sm text-white focus:outline-none focus:border-[#10b981] transition-colors"
            />
          </div>

          {error && (
            <p role="alert" className="text-xs text-rose-400 bg-rose-400/10 border border-rose-400/20 rounded-lg px-3 py-2">
              {error}
            </p>
          )}

          <button
            type="submit"
            disabled={sending || !username.trim() || !password}
            className="mt-2 flex items-center justify-center gap-2 bg-[#10b981]/20 hover:bg-[#10b981]/30 border border-[#10b981]/50 text-[#10b981] font-bold text-xs uppercase tracking-widest py-3 rounded-lg transition-all disabled:opacity-40"
          >
            {sending ? <Loader2 size={14} className="animate-spin" /> : <LogIn size={14} />}
            {sending ? 'Entrando...' : 'Entrar'}
          </button>
        </form>

        <p className="text-[10px] text-center text-[#737373] mt-6 tracking-wider uppercase">
          Acesso restrito. Fale com o administrador para obter uma conta.
        </p>
      </div>
    </div>
  );
};

export default LoginView;
