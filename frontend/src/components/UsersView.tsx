import { useState, useEffect, useCallback, type FormEvent } from 'react';
import { Users, Trash2, Plus, KeyRound, Globe, X, ShieldOff } from 'lucide-react';
import { api, type Site, type UserRecord } from '../lib/api';
import { ROLE_LABELS, type Role, type SiteAccess } from '../lib/session';
import { relativeTime } from '../lib/format';
import { useSession } from './ui/session-context';
import { useDialog } from './ui/dialog-context';
import Select from './ui/Select';

const ROLE_OPTIONS: Role[] = ['viewer', 'operator', 'admin'];

// Mesma ordem do ROLE_OPTIONS, já com o rótulo em pt-BR da lista suspensa.
const ROLE_SELECT_OPTIONS = ROLE_OPTIONS.map((r) => ({ value: r, label: ROLE_LABELS[r] }));

const emptyForm = { username: '', password: '', role: 'viewer' as Role };

/**
 * Administração de contas do painel. Só o administrador chega aqui — a aba
 * some para os demais e o backend recusa com 403 de qualquer forma.
 */
const UsersView = () => {
  const session = useSession();
  const dialog = useDialog();
  const [users, setUsers] = useState<UserRecord[]>([]);
  const [sites, setSites] = useState<Site[]>([]);
  const [form, setForm] = useState({ ...emptyForm });
  const [accesses, setAccesses] = useState<SiteAccess[]>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    try {
      const [userList, siteList] = await Promise.all([api.users(), api.sites()]);
      setUsers(userList);
      setSites(siteList);
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const siteName = (id: number | null) =>
    id === null ? 'Global (todas)' : sites.find((s) => s.id === id)?.name ?? `Unidade ${id}`;

  const notifyError = (err: unknown, fallback: string) => {
    const raw = (err as Error).message;
    let message = fallback;
    try {
      message = (JSON.parse(raw) as { error?: string }).error ?? fallback;
    } catch {
      // Corpo não-JSON: fica com a mensagem genérica.
    }
    dialog.notify(message, 'error');
  };

  const handleCreate = async (e: FormEvent) => {
    e.preventDefault();
    try {
      await api.createUser({
        username: form.username.trim(),
        password: form.password,
        role: form.role,
        ...(accesses.length > 0 ? { accesses } : {}),
      });
      setForm({ ...emptyForm });
      setAccesses([]);
      load();
      dialog.notify(`Usuário "${form.username.trim()}" criado.`, 'success');
    } catch (err) {
      notifyError(err, 'Falha ao criar o usuário.');
    }
  };

  const handleToggleActive = async (user: UserRecord) => {
    if (user.active) {
      const confirmed = await dialog.confirm({
        title: `Desativar "${user.username}"?`,
        message: 'As sessões abertas são derrubadas na hora.',
        confirmLabel: 'Desativar',
        danger: true,
      });
      if (!confirmed) return;
    }
    try {
      await api.updateUser(user.id, { active: !user.active });
      load();
    } catch (err) {
      notifyError(err, 'Falha ao alterar o usuário.');
    }
  };

  const handleRoleChange = async (user: UserRecord, role: Role) => {
    try {
      await api.updateUser(user.id, { role });
      load();
      dialog.notify(`Papel de "${user.username}" agora é ${ROLE_LABELS[role]}.`, 'success');
    } catch (err) {
      notifyError(err, 'Falha ao trocar o papel.');
    }
  };

  const handleResetPassword = async (user: UserRecord) => {
    const password = await dialog.prompt({
      title: `Nova senha para "${user.username}"`,
      message: 'Mínimo de 10 caracteres. As sessões abertas são derrubadas.',
      placeholder: 'Nova senha',
      confirmLabel: 'Trocar senha',
    });
    if (!password) return;
    try {
      await api.updateUser(user.id, { password });
      dialog.notify('Senha trocada.', 'success');
    } catch (err) {
      notifyError(err, 'Falha ao trocar a senha.');
    }
  };

  const handleDelete = async (user: UserRecord) => {
    const confirmed = await dialog.confirm({
      title: `Remover "${user.username}"?`,
      message: 'A conta e os acessos por unidade são apagados.',
      confirmLabel: 'Remover',
      danger: true,
    });
    if (!confirmed) return;
    try {
      await api.deleteUser(user.id);
      load();
    } catch (err) {
      notifyError(err, 'Falha ao remover o usuário.');
    }
  };

  const addAccess = () =>
    setAccesses((prev) => [...prev, { site_id: null, role: 'viewer' }]);

  const updateAccess = (index: number, patch: Partial<SiteAccess>) =>
    setAccesses((prev) => prev.map((a, i) => (i === index ? { ...a, ...patch } : a)));

  const removeAccess = (index: number) =>
    setAccesses((prev) => prev.filter((_, i) => i !== index));

  // A aba some do menu, mas a tela ainda pode ser alcançada por estado antigo.
  if (session.role !== 'admin') {
    return (
      <div className="p-8 h-full flex flex-col items-center justify-center text-[#737373] gap-3">
        <ShieldOff size={32} className="opacity-40" />
        <p className="text-sm">Seu perfil não permite esta tela.</p>
      </div>
    );
  }

  const inputClass =
    'w-full bg-black/40 border border-white/10 rounded-lg px-4 py-2 text-sm text-white focus:outline-none focus:border-[#10b981] transition-colors';

  return (
    <div className="p-8">
      <div className="flex items-center gap-3 mb-2">
        <Users size={22} className="text-[#10b981]" />
        <h1 className="text-2xl font-light text-white">
          Usuários do <span className="font-bold">Painel</span>
        </h1>
      </div>
      <p className="text-[#737373] text-sm mb-8">
        Cada pessoa entra com a própria conta e um papel: Visualizador só vê, Suporte TI opera,
        Administrador gerencia contas e servidores. O acesso pode ser restrito por unidade.
      </p>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="glass-panel p-6 rounded-xl border border-white/5 bg-white/[0.02] col-span-1 h-fit">
          <h2 className="text-sm font-bold tracking-widest text-[#737373] uppercase mb-6">Nova Conta</h2>
          <form onSubmit={handleCreate} className="flex flex-col gap-4">
            <div>
              <label htmlFor="user-name" className="text-xs text-[#737373] block mb-1">Usuário</label>
              <input
                id="user-name"
                type="text"
                value={form.username}
                onChange={(e) => setForm({ ...form, username: e.target.value })}
                className={inputClass}
                placeholder="Ex: joao.silva"
                autoComplete="off"
                required
              />
            </div>
            <div>
              <label htmlFor="user-password" className="text-xs text-[#737373] block mb-1">Senha</label>
              <input
                id="user-password"
                type="password"
                value={form.password}
                onChange={(e) => setForm({ ...form, password: e.target.value })}
                className={inputClass}
                placeholder="Mínimo 10 caracteres"
                autoComplete="new-password"
                minLength={10}
                required
              />
            </div>
            <div>
              <label htmlFor="user-role" className="text-xs text-[#737373] block mb-1">Papel</label>
              <Select
                id="user-role"
                value={form.role}
                onChange={(v) => setForm({ ...form, role: v as Role })}
                options={ROLE_SELECT_OPTIONS}
              />
            </div>

            <div>
              <div className="flex items-center justify-between mb-1">
                <span className="text-xs text-[#737373]">Acessos por unidade</span>
                <button
                  type="button"
                  onClick={addAccess}
                  className="flex items-center gap-1 text-[10px] font-bold uppercase tracking-widest text-[#10b981]"
                >
                  <Plus size={11} />
                  Adicionar
                </button>
              </div>
              {accesses.length === 0 ? (
                <p className="text-[10px] text-[#737373] leading-relaxed">
                  Sem restrição: o papel acima vale para todas as unidades e para a infraestrutura.
                </p>
              ) : (
                <div className="flex flex-col gap-2">
                  {accesses.map((access, index) => (
                    <div key={index} className="flex items-center gap-2">
                      <Select
                        ariaLabel="Unidade do acesso"
                        className="flex-1"
                        value={access.site_id === null ? 'global' : String(access.site_id)}
                        onChange={(v) =>
                          updateAccess(index, { site_id: v === 'global' ? null : Number(v) })
                        }
                        options={[
                          { value: 'global', label: 'Global (todas)' },
                          ...sites.map((s) => ({ value: String(s.id), label: s.name })),
                        ]}
                      />
                      <Select
                        ariaLabel="Papel do acesso"
                        className="w-36"
                        value={access.role}
                        onChange={(v) => updateAccess(index, { role: v as Role })}
                        options={ROLE_SELECT_OPTIONS}
                      />
                      <button
                        type="button"
                        onClick={() => removeAccess(index)}
                        aria-label="Remover acesso"
                        className="p-1.5 text-[#737373] hover:text-rose-400 transition-colors"
                      >
                        <X size={14} />
                      </button>
                    </div>
                  ))}
                </div>
              )}
            </div>

            <button
              type="submit"
              className="mt-2 flex items-center justify-center gap-2 bg-[#10b981]/20 hover:bg-[#10b981]/30 border border-[#10b981]/50 text-[#10b981] font-bold text-xs uppercase tracking-widest py-3 rounded-lg transition-all"
            >
              <Plus size={14} />
              Criar Conta
            </button>
          </form>
        </div>

        <div className="glass-panel p-6 rounded-xl border border-white/5 bg-white/[0.02] col-span-2">
          <h2 className="text-sm font-bold tracking-widest text-[#737373] uppercase mb-6">
            Contas {users.length > 0 && <span className="text-[#10b981]">({users.length})</span>}
          </h2>

          {loading ? (
            <p className="text-sm text-[#737373]">Carregando...</p>
          ) : users.length === 0 ? (
            <p className="text-sm text-[#737373]">Nenhuma conta cadastrada.</p>
          ) : (
            <div className="overflow-x-auto custom-scrollbar">
              <table className="w-full text-sm text-left border-collapse">
                <thead className="text-[10px] text-[#737373] uppercase tracking-widest bg-[#0c0c0e]">
                  <tr>
                    <th className="py-3 px-4 rounded-l">Status</th>
                    <th className="py-3 px-4">Usuário</th>
                    <th className="py-3 px-4">Papel</th>
                    <th className="py-3 px-4">Acessos</th>
                    <th className="py-3 px-4">Último login</th>
                    <th className="py-3 px-4 text-right rounded-r">Ações</th>
                  </tr>
                </thead>
                <tbody>
                  {users.map((user) => (
                    <tr key={user.id} className="border-b border-white/[0.03] hover:bg-white/[0.05] transition-all">
                      <td className="py-4 px-4">
                        <button
                          onClick={() => handleToggleActive(user)}
                          className="flex items-center gap-2"
                          title={user.active ? 'Desativar conta' : 'Reativar conta'}
                        >
                          <span className={`w-2 h-2 rounded-full ${user.active ? 'bg-[#10b981]' : 'bg-[#737373]'}`} />
                          <span className={`text-[10px] font-bold tracking-widest uppercase ${user.active ? 'text-[#10b981]' : 'text-[#737373]'}`}>
                            {user.active ? 'Ativa' : 'Inativa'}
                          </span>
                        </button>
                      </td>
                      <td className="py-4 px-4 font-medium text-white/90">
                        {user.username}
                        {user.username === session.username && (
                          <span className="ml-2 text-[10px] text-[#737373] uppercase tracking-wider">(você)</span>
                        )}
                      </td>
                      <td className="py-4 px-4">
                        <Select
                          ariaLabel={`Papel de ${user.username}`}
                          className="w-40"
                          value={user.role}
                          onChange={(v) => handleRoleChange(user, v as Role)}
                          options={ROLE_SELECT_OPTIONS}
                        />
                      </td>
                      <td className="py-4 px-4">
                        <div className="flex gap-1 flex-wrap">
                          {user.accesses.length === 0 ? (
                            <span className="inline-flex items-center gap-1 text-[10px] text-[#737373] bg-white/5 px-2 py-0.5 rounded border border-white/5">
                              <Globe size={10} />
                              Global (todas)
                            </span>
                          ) : (
                            user.accesses.map((a, i) => (
                              <span
                                key={`${a.site_id ?? 'global'}-${i}`}
                                className="text-[10px] text-gray-400 bg-white/5 px-2 py-0.5 rounded border border-white/5"
                              >
                                {siteName(a.site_id)}: {ROLE_LABELS[a.role]}
                              </span>
                            ))
                          )}
                        </div>
                      </td>
                      <td className="py-4 px-4 text-[#737373] text-xs">{relativeTime(user.last_login)}</td>
                      <td className="py-4 px-4 text-right whitespace-nowrap">
                        <button
                          onClick={() => handleResetPassword(user)}
                          title="Trocar senha"
                          className="p-1.5 text-gray-400 hover:text-[#10b981] hover:bg-[#10b981]/10 rounded transition-colors"
                        >
                          <KeyRound size={14} />
                        </button>
                        <button
                          onClick={() => handleDelete(user)}
                          title="Remover conta"
                          className="p-1.5 text-gray-400 hover:text-rose-400 hover:bg-rose-400/10 rounded transition-colors"
                        >
                          <Trash2 size={14} />
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

export default UsersView;
