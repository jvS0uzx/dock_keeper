import { useCallback, useEffect, useMemo, useState, type FormEvent } from 'react';
import {
  Check, Copy, KeyRound, MonitorSmartphone, Radar, ShieldOff, Ban, type LucideIcon,
} from 'lucide-react';
import { api, apiErrorMessage as messageOf, type DeviceKind, type DeviceRecord, type EnrollToken } from '../lib/api';
import { formatDateTime, relativeTime } from '../lib/format';
import { hasGlobalAdmin } from '../lib/panels';
import { useSession } from './ui/session-context';
import { useDialog } from './ui/dialog-context';
import { useSiteScope } from './ui/site-scope-context';
import Select from './ui/Select';

const KINDS: Record<DeviceKind, { label: string; icon: LucideIcon; envVar: string; envFile: string }> = {
  agent: {
    label: 'Agente de estação',
    icon: MonitorSmartphone,
    envVar: 'AGENT_ENROLL_TOKEN',
    envFile: '/etc/dockkeeper-agent.env',
  },
  collector: {
    label: 'Coletor de rede',
    icon: Radar,
    envVar: 'COLLECTOR_ENROLL_TOKEN',
    envFile: '/etc/dockkeeper-collector.env',
  },
};

const KIND_ORDER: DeviceKind[] = ['agent', 'collector'];

const DevicesView = () => {
  const session = useSession();
  const dialog = useDialog();
  const { sites, siteName, numericSiteId } = useSiteScope();
  const isGlobalAdmin = hasGlobalAdmin(session.accesses);

  const [devices, setDevices] = useState<DeviceRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [kind, setKind] = useState<DeviceKind>('agent');
  const [siteId, setSiteId] = useState('');
  const [issuing, setIssuing] = useState(false);
  const [issued, setIssued] = useState<EnrollToken | null>(null);
  const [copied, setCopied] = useState(false);

  const load = useCallback(async () => {
    try {
      setDevices(await api.devices());
      setLoadError(null);
    } catch (err) {
      setLoadError(messageOf(err, 'Falha ao listar os dispositivos.'));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (isGlobalAdmin) load();
  }, [isGlobalAdmin, load]);

  const inviteSite = siteId || String(numericSiteId ?? sites[0]?.id ?? '');

  const visible = useMemo(
    () => (numericSiteId === null ? devices : devices.filter((d) => d.site_id === numericSiteId)),
    [devices, numericSiteId],
  );

  const handleIssue = async (e: FormEvent) => {
    e.preventDefault();
    if (!inviteSite) return;
    setIssuing(true);
    try {
      setIssued(await api.createEnrollToken({ kind, site_id: Number(inviteSite) }));
      setCopied(false);
    } catch (err) {
      dialog.notify(messageOf(err, 'Falha ao emitir o convite.'), 'error');
    } finally {
      setIssuing(false);
    }
  };

  const handleCopy = async () => {
    if (!issued) return;
    try {
      await navigator.clipboard.writeText(issued.enrollment_token);
      setCopied(true);
    } catch {
      dialog.notify('Não foi possível copiar. Selecione o convite e copie manualmente.', 'error');
    }
  };

  const handleRevoke = async (device: DeviceRecord) => {
    const name = device.hostname || device.device_id;
    const confirmed = await dialog.confirm({
      title: `Revogar "${name}"?`,
      message:
        'O dispositivo perde o acesso na hora e só volta com um convite novo. Os outros dispositivos continuam funcionando.',
      confirmLabel: 'Revogar',
      danger: true,
    });
    if (!confirmed) return;
    try {
      await api.revokeDevice(device.device_id);
      dialog.notify(`"${name}" revogado.`, 'success');
      load();
    } catch (err) {
      dialog.notify(messageOf(err, 'Falha ao revogar o dispositivo.'), 'error');
    }
  };

  if (!isGlobalAdmin) {
    return (
      <div className="p-8 h-full flex flex-col items-center justify-center text-text-mut gap-3">
        <ShieldOff size={32} strokeWidth={1.75} className="opacity-40" />
        <p className="text-sm">Seu perfil não permite esta tela.</p>
      </div>
    );
  }

  const issuedKind = issued ? KINDS[issued.kind] : null;

  return (
    <div className="p-4 md:p-8 anim-rise">
      <div className="page-header">
        <div>
          <h1 className="page-title">Dispositivos</h1>
          <p className="page-desc">
            Agentes de estação e coletores de rede com credencial própria. Um convite cadastra um
            dispositivo novo; revogar corta o acesso de um só, sem afetar os outros.
          </p>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="panel p-5 col-span-1 h-fit">
          <h2 className="eyebrow mb-5">Novo convite</h2>
          <form onSubmit={handleIssue} className="flex flex-col gap-4">
            <div>
              <span className="eyebrow block mb-1.5">Tipo</span>
              <div className="grid grid-cols-2 gap-1 rounded-ctrl border border-line bg-ink-850 p-1">
                {KIND_ORDER.map((k) => {
                  const Icon = KINDS[k].icon;
                  const active = kind === k;
                  return (
                    <button
                      key={k}
                      type="button"
                      aria-pressed={active}
                      onClick={() => setKind(k)}
                      className={`flex items-center justify-center gap-1.5 rounded-[0.4rem] px-2 py-2 text-xs font-semibold transition-colors ${
                        active ? 'bg-ink-750 text-text-hi' : 'text-text-mut hover:text-text-hi'
                      }`}
                    >
                      <Icon size={14} strokeWidth={1.75} className={active ? 'text-accent' : 'text-text-faint'} />
                      {KINDS[k].label}
                    </button>
                  );
                })}
              </div>
            </div>

            <div>
              <label htmlFor="device-site" className="eyebrow block mb-1.5">Unidade</label>
              <Select
                id="device-site"
                ariaLabel="Unidade do convite"
                value={inviteSite}
                onChange={setSiteId}
                placeholder="Nenhuma unidade cadastrada"
                disabled={sites.length === 0}
                options={sites.map((s) => ({ value: String(s.id), label: s.name }))}
              />
              <p className="text-[11px] text-text-faint mt-1 leading-relaxed">
                A unidade fica gravada na credencial: o dispositivo não consegue declarar outra.
              </p>
            </div>

            <button type="submit" className="btn btn-primary mt-1" disabled={issuing || !inviteSite}>
              <KeyRound size={16} strokeWidth={1.75} />
              {issuing ? 'Emitindo...' : 'Emitir convite'}
            </button>
          </form>

          {issued && issuedKind && (
            <div className="mt-5 rounded-ctrl border border-accent/40 bg-accent/5 p-4 flex flex-col gap-3">
              <p className="text-sm font-medium text-text-hi">
                Copie agora: este convite não aparece de novo.
              </p>
              <div className="mono-data selectable break-all rounded-ctrl border border-line bg-ink-850 px-3 py-2 text-sm text-text-hi">
                {issued.enrollment_token}
              </div>
              <p className="text-xs text-text-mut">
                Vale até {formatDateTime(issued.expires_at)}, para um único {issuedKind.label.toLowerCase()} em{' '}
                {siteName(issued.site_id)}.
              </p>
              <p className="text-xs text-text-mut leading-relaxed">
                Cole em <code className="mono-data text-text">{issuedKind.envVar}</code> no arquivo{' '}
                <code className="mono-data text-text">{issuedKind.envFile}</code> e reinicie o serviço.
              </p>
              <div className="flex flex-wrap gap-2">
                <button type="button" onClick={handleCopy} className="btn btn-ghost btn-sm">
                  {copied ? <Check size={14} strokeWidth={1.75} className="text-ok" /> : <Copy size={14} strokeWidth={1.75} />}
                  {copied ? 'Copiado' : 'Copiar convite'}
                </button>
                <button type="button" onClick={() => setIssued(null)} className="btn btn-ghost btn-sm">
                  Já guardei o convite
                </button>
              </div>
            </div>
          )}
        </div>

        <div className="panel p-5 col-span-1 lg:col-span-2">
          <h2 className="eyebrow mb-4">
            Cadastrados{visible.length > 0 && <span className="mono-data text-text-mut normal-case tracking-normal"> · {visible.length}</span>}
          </h2>

          {loading ? (
            <p className="text-sm text-text-mut">Carregando...</p>
          ) : loadError ? (
            <p role="alert" className="text-sm text-crit">{loadError}</p>
          ) : visible.length === 0 ? (
            <p className="text-sm text-text-mut">
              Nenhum dispositivo cadastrado{numericSiteId === null ? '' : ' nesta unidade'}. Emita um convite
              para cadastrar o primeiro.
            </p>
          ) : (
            <div className="overflow-x-auto custom-scrollbar">
              <table className="table-base min-w-[760px]">
                <thead>
                  <tr>
                    <th>Dispositivo</th>
                    <th>Tipo</th>
                    <th>Unidade</th>
                    <th>Última atividade</th>
                    <th>Estado</th>
                    <th className="text-right">Ações</th>
                  </tr>
                </thead>
                <tbody>
                  {visible.map((device) => {
                    const meta = KINDS[device.kind] ?? KINDS.agent;
                    const Icon = meta.icon;
                    const revoked = device.revoked_at !== null;
                    return (
                      <tr key={device.device_id}>
                        <td>
                          <div className={`mono-data selectable text-sm ${revoked ? 'text-text-mut' : 'text-text-hi'}`}>
                            {device.hostname || device.device_id}
                          </div>
                          {device.hostname && (
                            <div className="mono-data selectable text-[11px] text-text-faint">{device.device_id}</div>
                          )}
                        </td>
                        <td>
                          <span className="inline-flex items-center gap-1.5 text-text-mut">
                            <Icon size={14} strokeWidth={1.75} className="text-text-faint" />
                            <span>{meta.label}</span>
                          </span>
                        </td>
                        <td className="text-text-mut">{siteName(device.site_id)}</td>
                        <td
                          className={device.last_seen_at ? 'text-text-mut text-xs' : 'text-text-faint text-xs'}
                          title={device.last_seen_at ? formatDateTime(device.last_seen_at) : 'Nunca enviou dados'}
                        >
                          {relativeTime(device.last_seen_at)}
                        </td>
                        <td>
                          {revoked ? (
                            <span
                              className="badge badge-muted"
                              title={`Revogado em ${formatDateTime(device.revoked_at as string)}`}
                            >
                              Revogado
                            </span>
                          ) : (
                            <span className="badge badge-ok">Ativo</span>
                          )}
                        </td>
                        <td className="text-right">
                          {!revoked && (
                            <button
                              onClick={() => handleRevoke(device)}
                              className="inline-flex items-center gap-1.5 text-xs text-crit/80 hover:text-crit transition-colors"
                            >
                              <Ban size={14} strokeWidth={1.75} />
                              Revogar
                            </button>
                          )}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

export default DevicesView;
