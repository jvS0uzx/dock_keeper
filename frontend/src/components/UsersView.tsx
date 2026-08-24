import { useState, useEffect, useCallback, type FormEvent } from 'react';
import { Trash2, Plus, KeyRound, Globe, X, ShieldOff } from 'lucide-react';
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
      <div className="p-8 h-full flex flex-col items-center justify-center text-text-mut gap-3">
        <ShieldOff size={32} strokeWidth={1.75} className="opacity-40" />
        <p className="text-sm">Seu perfil não permite esta tela.</p>
      </div>
    );
  }

  return (
    <div className="p-8 anim-rise">
      <div className="page-header">
        <div>
          <h1 className="page-title">Usuários do painel</h1>
          <p className="page-desc">
            Cada pessoa entra com a própria conta e um papel: Visualizador só vê, Suporte TI opera,
            Administrador gerencia contas e servidores. O acesso pode ser restrito por unidade.
          </p>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="panel p-5 col-span-1 h-fit">
          <h2 className="eyebrow mb-5">Nova conta</h2>
          <form onSubmit={handleCreate} className="flex flex-col gap-4">
            <div>
              <label htmlFor="user-name" className="eyebrow block mb-1.5">Usuário</label>
              <input
                id="user-name"
                type="text"
                value={form.username}
                onChange={(e) => setForm({ ...form, username: e.target.value })}
                className="input-base w-full"
                placeholder="Ex: joao.silva"
                autoComplete="off"
                required
              />
            </div>
            <div>
              <label htmlFor="user-password" className="eyebrow block mb-1.5">Senha</label>
              <input
                id="user-password"
                type="password"
                value={form.password}
                onChange={(e) => setForm({ ...form, password: e.target.value })}
                className="input-base w-full"
                placeholder="Mínimo 10 caracteres"
                autoComplete="new-password"
                minLength={10}
                required
              />
            </div>
            <div>
              <label htmlFor="user-role" className="eyebrow block mb-1.5">Papel</label>
              <Select
                id="user-role"
                value={form.role}
                onChange={(v) => setForm({ ...form, role: v as Role })}
                options={ROLE_SELECT_OPTIONS}
              />
            </div>

            <div>
              <div className="flex items-center justify-between mb-1.5">
                <span className="eyebrow">Acessos por unidade</span>
                <button
                  type="button"
                  onClick={addAccess}
                  className="btn btn-ghost btn-sm px-1.5 text-accent"
                >
                  <Plus size={12} strokeWidth={1.75} />
                  Adicionar
                </button>
              </div>
              {accesses.length === 0 ? (
                <p className="text-[11px] text-text-faint leading-relaxed">
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
                        className="btn btn-ghost btn-sm px-1.5 hover:text-crit"
                      >
                        <X size={14} strokeWidth={1.75} />
                      </button>
                    </div>
                  ))}
                </div>
              )}
            </div>

            <button type="submit" className="btn btn-primary mt-1">
              <Plus size={16} strokeWidth={1.75} />
              Criar conta
            </button>
          </form>
        </div>

        <div className="panel p-5 col-span-2">
          <h2 className="eyebrow mb-4">
            Contas{users.length > 0 && <span className="mono-data text-text-mut normal-case tracking-normal"> · {users.length}</span>}
          </h2>

          {loading ? (
            <p className="text-sm text-text-mut">Carregando...</p>
          ) : users.length === 0 ? (
            <p className="text-sm text-text-mut">Nenhuma conta cadastrada.</p>
          ) : (
            <div className="overflow-x-auto custom-scrollbar">
              <table className="table-base min-w-[720px]">
                <thead>
                  <tr>
                    <th>Status</th>
                    <th>Usuário</th>
                    <th>Papel</th>
                    <th>Acessos</th>
                    <th>Último login</th>
                    <th className="text-right">Ações</th>
                  </tr>
                </thead>
                <tbody>
                  {users.map((user) => (
                    <tr key={user.id}>
                      <td>
                        <button
                          onClick={() => handleToggleActive(user)}
                          className="inline-flex"
                          title={user.active ? 'Desativar conta' : 'Reativar conta'}
                        >
                          <span className={`badge ${user.active ? 'badge-ok' : 'badge-muted'}`}>
                            {user.active ? 'Ativa' : 'Inativa'}
                          </span>
                        </button>
                      </td>
                      <td className="font-medium text-text-hi">
                        {user.username}
                        {user.username === session.username && (
                          <span className="ml-2 text-[11px] text-text-faint">(você)</span>
                        )}
                      </td>
                      <td>
                        <Select
                          ariaLabel={`Papel de ${user.username}`}
                          className="w-40"
                          value={user.role}
                          onChange={(v) => handleRoleChange(user, v as Role)}
                          options={ROLE_SELECT_OPTIONS}
                        />
                      </td>
                      <td>
                        <div className="flex gap-1 flex-wrap">
                          {user.accesses.length === 0 ? (
                            <span className="badge badge-muted">
                              <Globe size={10} strokeWidth={1.75} />
                              Global (todas)
                            </span>
                          ) : (
                            user.accesses.map((a, i) => (
                              <span key={`${a.site_id ?? 'global'}-${i}`} className="badge badge-muted">
                                {siteName(a.site_id)}: {ROLE_LABELS[a.role]}
                              </span>
                            ))
                          )}
                        </div>
                      </td>
                      <td className="text-text-faint text-xs">{relativeTime(user.last_login)}</td>
                      <td className="text-right whitespace-nowrap">
                        <button
                          onClick={() => handleResetPassword(user)}
                          title="Trocar senha"
                          className="btn btn-ghost btn-sm"
                        >
                          <KeyRound size={14} strokeWidth={1.75} />
                        </button>
                        <button
                          onClick={() => handleDelete(user)}
                          title="Remover conta"
                          className="btn btn-ghost btn-sm ml-1.5 hover:text-crit"
                        >
                          <Trash2 size={14} strokeWidth={1.75} />
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
