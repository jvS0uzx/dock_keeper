import { useCallback, useEffect, useMemo, useState } from 'react';
import { BellRing, CheckCircle2, Clock, Send, XCircle } from 'lucide-react';

import { api, type AlertDelivery, type AlertItem, type AlertStatus } from '../lib/api';
import { formatDateTime, relativeTime } from '../lib/format';
import { useDialog } from './ui/dialog-context';
import { useSiteScope } from './ui/site-scope-context';
import LoadNotice from './ui/LoadNotice';
import { useLoadStatus } from './ui/load-status';
import Select from './ui/Select';

const INTERVALO_MS = 15000;

const ESTADOS: { value: AlertStatus | 'all'; label: string }[] = [
  { value: 'open', label: 'Abertos' },
  { value: 'acked', label: 'Reconhecidos' },
  { value: 'resolved', label: 'Resolvidos' },
  { value: 'all', label: 'Todos' },
];

const SEVERIDADES = [
  { value: 'all', label: 'Todas' },
  { value: 'critical', label: 'Crítica' },
  { value: 'warning', label: 'Atenção' },
  { value: 'info', label: 'Informativa' },
];

const SEVERIDADE_CLASSE: Record<string, string> = {
  critical: 'badge badge-crit',
  warning: 'badge badge-warn',
  info: 'badge',
};

const SEVERIDADE_ROTULO: Record<string, string> = {
  critical: 'Crítica',
  warning: 'Atenção',
  info: 'Informativa',
};

const ESTADO_ROTULO: Record<AlertStatus, string> = {
  open: 'Aberto',
  acked: 'Reconhecido',
  resolved: 'Resolvido',
};

const ENTREGA: Record<AlertDelivery, { rotulo: string; classe: string; Icon: typeof Send }> = {
  enviado: { rotulo: 'Enviado', classe: 'text-ok', Icon: CheckCircle2 },
  pendente: { rotulo: 'Pendente', classe: 'text-warn', Icon: Clock },
  falhou: { rotulo: 'Falhou', classe: 'text-crit', Icon: XCircle },
};

const AlertsView = () => {
  const dialog = useDialog();
  const { siteName } = useSiteScope();
  const [alertas, setAlertas] = useState<AlertItem[]>([]);
  const [estado, setEstado] = useState<AlertStatus | 'all'>('open');
  const [severidade, setSeveridade] = useState('all');
  const [carregando, setCarregando] = useState(true);
  const { error, lastOk, ok, fail } = useLoadStatus();

  const carregar = useCallback(
    async (signal?: AbortSignal) => {
      try {
        const lista = await api.alerts({ status: estado }, signal);
        setAlertas(lista);
        ok();
      } catch (err) {
        if (signal?.aborted) return;
        fail(err, 'Falha ao carregar os alertas.');
      } finally {
        setCarregando(false);
      }
    },
    [estado, ok, fail],
  );

  useEffect(() => {
    const controle = new AbortController();
    setCarregando(true);
    carregar(controle.signal);
    const timer = setInterval(() => carregar(), INTERVALO_MS);
    return () => {
      controle.abort();
      clearInterval(timer);
    };
  }, [carregar]);

  const visiveis = useMemo(
    () => (severidade === 'all' ? alertas : alertas.filter((a) => a.severity === severidade)),
    [alertas, severidade],
  );

  const agir = async (alerta: AlertItem, acao: 'ack' | 'resolve') => {
    const rotulo = acao === 'ack' ? 'Reconhecer' : 'Resolver';
    const confirmado = await dialog.confirm({
      title: `${rotulo} o alerta?`,
      message: alerta.text,
      confirmLabel: rotulo,
    });
    if (!confirmado) return;

    try {
      if (acao === 'ack') await api.ackAlert(alerta.id);
      else await api.resolveAlert(alerta.id);
      dialog.notify(`Alerta ${acao === 'ack' ? 'reconhecido' : 'resolvido'}.`, 'success');
      await carregar();
    } catch (err) {
      dialog.notify(
        err instanceof Error ? JSON.parse(err.message).error ?? `Falha ao ${rotulo.toLowerCase()}.` : `Falha ao ${rotulo.toLowerCase()}.`,
        'error',
      );
    }
  };

  const origem = (alerta: AlertItem) => {
    const partes = [alerta.server_id, alerta.site_id === null ? null : siteName(alerta.site_id)].filter(Boolean);
    return partes.length === 0 ? '—' : partes.join(' · ');
  };

  return (
    <div className="p-4 md:p-8 h-full flex flex-col overflow-hidden anim-rise">
      <div className="page-header flex-col md:flex-row md:items-end items-start">
        <div>
          <h1 className="page-title">Alertas</h1>
          <p className="page-desc">
            Cada disparo vira um alerta com ciclo próprio. A coluna de entrega mostra se o aviso
            chegou ao Telegram: pendente ou falhou significa que ninguém foi avisado ainda.
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-3">
          <div className="flex items-center gap-2">
            <label htmlFor="alerta-estado" className="eyebrow">Estado</label>
            <Select
              id="alerta-estado"
              value={estado}
              onChange={(v) => setEstado(v as AlertStatus | 'all')}
              options={ESTADOS}
            />
          </div>
          <div className="flex items-center gap-2">
            <label htmlFor="alerta-severidade" className="eyebrow">Severidade</label>
            <Select
              id="alerta-severidade"
              value={severidade}
              onChange={setSeveridade}
              options={SEVERIDADES}
            />
          </div>
        </div>
      </div>

      <LoadNotice error={error} lastOk={lastOk} className="mb-4" />

      <div className="panel flex-1 min-h-0 overflow-auto custom-scrollbar">
        <table className="data-table">
          <thead>
            <tr>
              <th>Severidade</th>
              <th>Alerta</th>
              <th>Origem</th>
              <th>Abriu</th>
              <th>Estado</th>
              <th>Entrega</th>
              <th className="text-right">Ações</th>
            </tr>
          </thead>
          <tbody>
            {visiveis.map((alerta) => {
              const entrega = ENTREGA[alerta.delivery];
              const EntregaIcon = entrega.Icon;
              return (
                <tr key={alerta.id}>
                  <td>
                    <span className={SEVERIDADE_CLASSE[alerta.severity] ?? 'badge'}>
                      {SEVERIDADE_ROTULO[alerta.severity] ?? alerta.severity}
                    </span>
                  </td>
                  <td className="text-text-hi">{alerta.text}</td>
                  <td className="mono-data text-text-mut">{origem(alerta)}</td>
                  <td className="text-text-mut" title={formatDateTime(alerta.created_at)}>
                    {relativeTime(alerta.created_at)}
                  </td>
                  <td>
                    <span className="text-text-mut">{ESTADO_ROTULO[alerta.status]}</span>
                    {alerta.acked_by && (
                      <span className="block text-xs text-text-faint">por {alerta.acked_by}</span>
                    )}
                  </td>
                  <td>
                    <span className={`flex items-center gap-1.5 ${entrega.classe}`}>
                      <EntregaIcon size={14} strokeWidth={1.75} />
                      {entrega.rotulo}
                    </span>
                    {alerta.delivery !== 'enviado' && (
                      <span className="block text-xs text-text-faint">
                        {alerta.attempts} {alerta.attempts === 1 ? 'tentativa' : 'tentativas'}
                        {alerta.last_error ? `: ${alerta.last_error}` : ''}
                      </span>
                    )}
                  </td>
                  <td className="text-right whitespace-nowrap">
                    {alerta.status === 'open' && (
                      <button className="btn btn-ghost btn-sm" onClick={() => agir(alerta, 'ack')}>
                        Reconhecer
                      </button>
                    )}
                    {alerta.status !== 'resolved' && (
                      <button className="btn btn-ghost btn-sm" onClick={() => agir(alerta, 'resolve')}>
                        Resolver
                      </button>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>

        {visiveis.length === 0 && (
          <div className="flex flex-col items-center justify-center gap-2 py-16 text-text-faint">
            {carregando && <span>Carregando alertas...</span>}
            {!carregando && !error && (
              <>
                <BellRing size={20} strokeWidth={1.5} />
                <span>Nenhum alerta neste filtro.</span>
              </>
            )}
          </div>
        )}
      </div>
    </div>
  );
};

export default AlertsView;
