import { useEffect, useState } from 'react';
import {
  ResponsiveContainer, LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, Legend,
} from 'recharts';
import { AlertTriangle } from 'lucide-react';
import {
  api,
  apiErrorMessage,
  type InterfacesDoHost,
  type JanelaDaSerie,
  type PontoDeInterface,
} from '../lib/api';
import {
  formatBps, formatContador, formatDateTime, formatUptime, formatVelocidade, relativeTime,
} from '../lib/format';
import { POLL } from '../lib/polling';
import { marcaDaInterface, nomeDaInterface } from '../lib/snmp';

const JANELAS: JanelaDaSerie[] = ['1h', '6h', '24h', '72h'];

const ParDeContadores = ({ entrada, saida, testid }: { entrada: number | null | undefined; saida: number | null | undefined; testid: string }) => (
  <span className="mono-data text-xs" data-testid={testid}>
    <span className={entrada ? 'text-warn' : 'text-text-mut'}>{formatContador(entrada)}</span>
    <span className="text-text-faint"> / </span>
    <span className={saida ? 'text-warn' : 'text-text-mut'}>{formatContador(saida)}</span>
  </span>
);

const DIA_MS = 24 * 60 * 60 * 1000;

const rotuloDoEixo = (spanMs: number) => (ms: number) => {
  const d = new Date(ms);
  if (spanMs > DIA_MS) {
    return d.toLocaleString('pt-BR', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' });
  }
  return d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
};

export const GraficoDeInterface = ({ interfaceId, nome }: { interfaceId: number; nome: string }) => {
  const [janela, setJanela] = useState<JanelaDaSerie>('1h');
  const [pontos, setPontos] = useState<PontoDeInterface[]>([]);
  const [carregadoPara, setCarregadoPara] = useState<string | null>(null);
  const chave = `${interfaceId}|${janela}`;
  const carregando = carregadoPara !== chave;
  const [erro, setErro] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    const carregar = async () => {
      try {
        setPontos(await api.serieDaInterface(interfaceId, janela, controller.signal));
        setErro(null);
      } catch (err) {
        if (controller.signal.aborted) return;
        setPontos([]);
        setErro(apiErrorMessage(err, 'Falha ao carregar o tráfego da interface.'));
      } finally {
        if (!controller.signal.aborted) setCarregadoPara(`${interfaceId}|${janela}`);
      }
    };
    carregar();
    const intervalo = setInterval(carregar, POLL.serieDoGrafico);
    return () => {
      clearInterval(intervalo);
      controller.abort();
    };
  }, [interfaceId, janela]);

  const dados = pontos.map((p) => ({ t: new Date(p.ts).getTime(), entrada: p.in_bps, saida: p.out_bps }));
  const span = dados.length > 1 ? dados[dados.length - 1].t - dados[0].t : 0;

  return (
    <section className="p-5" data-testid="grafico-da-interface">
      <div className="mb-4 flex items-center justify-between gap-3">
        <h3 className="eyebrow truncate">Tráfego de {nome}</h3>
        <div className="flex items-center gap-1" role="group" aria-label="Janela do gráfico">
          {JANELAS.map((j) => (
            <button
              key={j}
              type="button"
              onClick={() => setJanela(j)}
              aria-pressed={janela === j}
              className={`btn text-xs ${
                janela === j ? 'border border-accent/50 bg-accent/10 text-accent' : 'btn-ghost text-text-mut'
              }`}
            >
              {j}
            </button>
          ))}
        </div>
      </div>
      <div className="h-56">
        {erro ? (
          <div role="alert" className="flex h-full items-center justify-center gap-2 px-6 text-center text-sm text-crit">
            <AlertTriangle size={16} strokeWidth={1.75} />
            <span>{erro}</span>
          </div>
        ) : dados.length === 0 ? (
          <div className="flex h-full items-center justify-center px-6 text-center text-sm text-text-faint">
            {carregando ? 'Carregando...' : 'Sem leituras nesta janela.'}
          </div>
        ) : (
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={dados} margin={{ top: 8, right: 16, left: 0, bottom: 0 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="var(--color-line)" strokeOpacity={0.5} vertical={false} />
              <XAxis
                dataKey="t"
                type="number"
                scale="time"
                domain={['dataMin', 'dataMax']}
                tickFormatter={rotuloDoEixo(span)}
                tick={{ fill: 'var(--color-text-faint)', fontSize: 11 }}
                tickLine={false}
                axisLine={false}
                minTickGap={30}
              />
              <YAxis
                tick={{ fill: 'var(--color-text-faint)', fontSize: 11 }}
                tickLine={false}
                axisLine={false}
                width={76}
                tickFormatter={(v: number) => formatBps(v)}
              />
              <Tooltip
                contentStyle={{
                  background: 'var(--color-ink-800)',
                  border: '1px solid var(--color-line-hi)',
                  borderRadius: 10,
                  fontSize: 12,
                  fontFamily: 'var(--font-mono)',
                }}
                labelStyle={{ color: 'var(--color-text-mut)' }}
                labelFormatter={(ms) => formatDateTime(new Date(Number(ms)).toISOString())}
                formatter={(value, rotulo) => [formatBps(value === null ? null : Number(value)), rotulo]}
              />
              <Legend wrapperStyle={{ fontSize: 11 }} />
              <Line
                type="monotone"
                dataKey="entrada"
                name="Entrada"
                stroke="var(--color-accent)"
                strokeWidth={2}
                dot={false}
                isAnimationActive={false}
              />
              <Line
                type="monotone"
                dataKey="saida"
                name="Saída"
                stroke="var(--color-info)"
                strokeWidth={2}
                dot={false}
                isAnimationActive={false}
              />
            </LineChart>
          </ResponsiveContainer>
        )}
      </div>
    </section>
  );
};

export const AbaDeInterfaces = ({ hostId }: { hostId: number }) => {
  const [dados, setDados] = useState<InterfacesDoHost | null>(null);
  const [erro, setErro] = useState<string | null>(null);
  const [selecionada, setSelecionada] = useState<number | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    const carregar = async () => {
      try {
        setDados(await api.interfacesDoHost(hostId, controller.signal));
        setErro(null);
      } catch (err) {
        if (!controller.signal.aborted) setErro(apiErrorMessage(err, 'Falha ao ler as interfaces.'));
      }
    };
    carregar();
    const intervalo = setInterval(carregar, POLL.interfacesDeRede);
    return () => {
      clearInterval(intervalo);
      controller.abort();
    };
  }, [hostId]);

  if (dados === null) {
    return (
      <div className="p-5 text-sm text-text-faint">
        {erro ? <span role="alert" className="text-crit">{erro}</span> : 'Carregando interfaces...'}
      </div>
    );
  }

  const { host, interfaces } = dados;
  const escolhida = interfaces.find((itf) => itf.id === selecionada) ?? null;

  return (
    <>
      <section className="border-b border-line p-5" data-testid="cabecalho-snmp">
        <h3 className="text-base font-semibold text-text-hi" data-testid="snmp-sys-name">
          {host.snmp_sys_name || '—'}
        </h3>
        <p className="mt-1 text-xs text-text-mut" data-testid="snmp-sys-descr">{host.snmp_sys_descr || '—'}</p>
        <div className="mt-3 flex flex-wrap gap-x-5 gap-y-1 text-xs text-text-mut">
          <span>
            Ligado há <span className="mono-data text-text-hi" data-testid="snmp-uptime">{formatUptime(host.snmp_uptime_sec)}</span>
          </span>
          <span data-testid="snmp-visto" title={host.snmp_visto_em ? formatDateTime(host.snmp_visto_em) : undefined}>
            {host.snmp_visto_em ? `Visto ${relativeTime(host.snmp_visto_em)}` : 'Nunca respondeu ao SNMP'}
          </span>
        </div>
        {host.snmp_erro && (
          <p role="alert" className="mt-3 flex items-center gap-2 text-xs text-crit" data-testid="snmp-erro">
            <AlertTriangle size={14} strokeWidth={1.75} />
            <span>
              Última coleta falhou{host.snmp_erro_em ? ` ${relativeTime(host.snmp_erro_em)}` : ''}: {host.snmp_erro}
            </span>
          </p>
        )}
        {erro && <p role="alert" className="mt-3 text-xs text-warn">{erro}</p>}
      </section>

      <section className="border-b border-line">
        {interfaces.length === 0 ? (
          <p className="p-5 text-sm text-text-mut">Nenhuma interface coletada ainda.</p>
        ) : (
          <div className="overflow-x-auto custom-scrollbar">
            <table className="table-base whitespace-nowrap">
              <thead>
                <tr>
                  <th>Interface</th>
                  <th>Velocidade</th>
                  <th>Estado</th>
                  <th className="text-right">Entrada</th>
                  <th className="text-right">Saída</th>
                  <th className="text-right">Erros</th>
                  <th className="text-right">Descartes</th>
                </tr>
              </thead>
              <tbody>
                {interfaces.map((itf) => {
                  const nome = nomeDaInterface(itf);
                  const marca = marcaDaInterface(itf);
                  return (
                    <tr
                      key={itf.id}
                      data-testid="interface"
                      className={selecionada === itf.id ? 'bg-accent/5' : undefined}
                    >
                      <td>
                        <button
                          type="button"
                          onClick={() => setSelecionada(itf.id)}
                          aria-label={`Ver tráfego de ${nome}`}
                          className="flex flex-col items-start text-left"
                        >
                          <span className="mono-data text-sm text-text-hi hover:text-accent">{nome}</span>
                          {itf.if_alias && <span className="text-[11px] text-text-faint">{itf.if_alias}</span>}
                        </button>
                      </td>
                      <td className="mono-data text-xs text-text-mut" data-testid="interface-velocidade">
                        {formatVelocidade(itf.speed_mbps)}
                      </td>
                      <td>
                        <span className={`badge ${marca.classe}`} data-testid="interface-estado">{marca.texto}</span>
                      </td>
                      <td className="mono-data text-right text-xs text-text-hi" data-testid="interface-entrada">
                        {formatBps(itf.ultima?.in_bps)}
                      </td>
                      <td className="mono-data text-right text-xs text-text-hi" data-testid="interface-saida">
                        {formatBps(itf.ultima?.out_bps)}
                      </td>
                      <td className="text-right">
                        <ParDeContadores entrada={itf.ultima?.in_errors} saida={itf.ultima?.out_errors} testid="interface-erros" />
                      </td>
                      <td className="text-right">
                        <ParDeContadores entrada={itf.ultima?.in_discards} saida={itf.ultima?.out_discards} testid="interface-descartes" />
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {escolhida !== null ? (
        <GraficoDeInterface key={escolhida.id} interfaceId={escolhida.id} nome={nomeDaInterface(escolhida)} />
      ) : (
        interfaces.length > 0 && (
          <p className="p-5 text-xs text-text-faint">Escolha uma interface para ver o tráfego no tempo.</p>
        )
      )}
    </>
  );
};
