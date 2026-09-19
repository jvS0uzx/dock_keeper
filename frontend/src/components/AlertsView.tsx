import { useCallback, useEffect, useMemo, useState } from 'react';
import { BellOff, BellRing, CheckCircle2, CircleDashed, Clock, Send, XCircle } from 'lucide-react';

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

interface Entrega {
  rotulo: string;
  classe: string;
  Icon: typeof Send;
}

const ENTREGA: Record<AlertDelivery, Entrega> = {
  enviado: { rotulo: 'Enviado', classe: 'text-ok', Icon: CheckCircle2 },
  pendente: { rotulo: 'Pendente', classe: 'text-warn', Icon: Clock },
  falhou: { rotulo: 'Falhou', classe: 'text-crit', Icon: XCircle },
  sem_canal: { rotulo: 'Sem canal', classe: 'text-info', Icon: BellOff },
};

const entregaDe = (valor: string): Entrega =>
  ENTREGA[valor as AlertDelivery] ?? { rotulo: valor, classe: 'text-text-faint', Icon: CircleDashed };

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
    return partes.join(' · ');
  };

  return (
    <div className="p-4 md:p-8 h-full flex flex-col overflow-hidden anim-rise">
      <div className="page-header flex-col md:flex-row md:items-end items-start">
        <div>
          <h1 className="page-title">Alertas</h1>
          <p className="page-desc">
            Cada disparo vira um alerta com ciclo próprio. O estado de entrega mostra se o aviso
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
        <ul className="flex flex-col">
          {visiveis.map((alerta) => {
            const entrega = entregaDe(alerta.delivery);
            const EntregaIcon = entrega.Icon;
            const partesOrigem = origem(alerta);
            return (
              <li
                key={alerta.id}
                className="flex flex-col gap-4 border-b border-line px-5 py-4 last:border-b-0 lg:flex-row lg:items-start lg:gap-6"
              >
                <div className="flex shrink-0 items-center gap-3 lg:w-36 lg:flex-col lg:items-start lg:gap-1.5">
                  <span className={SEVERIDADE_CLASSE[alerta.severity] ?? 'badge'}>
                    {SEVERIDADE_ROTULO[alerta.severity] ?? alerta.severity}
                  </span>
                  <span className="text-xs text-text-faint" title={formatDateTime(alerta.created_at)}>
                    {relativeTime(alerta.created_at)}
                  </span>
                </div>

                <div className="min-w-0 flex-1">
                  <p className="max-w-prose text-sm leading-relaxed text-text-hi">{alerta.text}</p>

                  <p className="mt-1.5 text-xs text-text-mut">
                    <span className="text-text-faint">{ESTADO_ROTULO[alerta.status]}</span>
                    {alerta.acked_by && <span className="text-text-faint"> por {alerta.acked_by}</span>}
                    {partesOrigem !== '' && (
                      <>
                        <span className="text-text-faint"> · </span>
                        <span className="mono-data">{partesOrigem}</span>
                      </>
                    )}
                  </p>

                  <div className="mt-2.5">
                    <span className={`inline-flex items-center gap-1.5 text-xs ${entrega.classe}`}>
                      <EntregaIcon size={14} strokeWidth={1.75} />
                      {entrega.rotulo}
                    </span>

                    {alerta.delivery !== 'enviado' && (
                      <details className="mt-1.5">
                        <summary className="w-fit cursor-pointer text-xs text-text-mut transition-colors hover:text-text">
                          Detalhe da entrega
                        </summary>
                        <div className="mt-2 flex flex-col gap-1 rounded-ctrl border border-line bg-ink-850 p-3 text-xs text-text-mut">
                          {alerta.delivery === 'sem_canal' && (
                            <span className="max-w-prose">
                              Não há canal de Telegram configurado. Ao configurar, todo alerta aberto com menos de 24 h
                              volta sozinho para a fila de entrega.
                            </span>
                          )}
                          {alerta.attempts > 0 && (
                            <span>
                              {alerta.attempts} {alerta.attempts === 1 ? 'tentativa' : 'tentativas'}
                            </span>
                          )}
                          {alerta.last_error && (
                            <span className="max-w-prose break-words text-text-faint">
                              Motivo: {alerta.last_error}
                            </span>
                          )}
                          {alerta.last_attempt_at && (
                            <span className="text-text-faint">
                              Última tentativa: {formatDateTime(alerta.last_attempt_at)}
                            </span>
                          )}
                          {alerta.next_attempt_at && alerta.delivery === 'pendente' && (
                            <span className="text-text-faint">
                              Próxima tentativa: {formatDateTime(alerta.next_attempt_at)}
                            </span>
                          )}
                        </div>
                      </details>
                    )}
                  </div>
                </div>

                <div className="flex shrink-0 items-center gap-3 lg:pt-0.5">
                  {alerta.status === 'open' && (
                    <button className="btn btn-ghost btn-sm" onClick={() => agir(alerta, 'ack')}>
                      Reconhecer
                    </button>
                  )}
                  {alerta.status !== 'resolved' && (
                    <button className="btn btn-primary btn-sm" onClick={() => agir(alerta, 'resolve')}>
                      Resolver
                    </button>
                  )}
                </div>
              </li>
            );
          })}
        </ul>

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
