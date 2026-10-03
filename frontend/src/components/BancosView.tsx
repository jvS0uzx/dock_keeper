import { useCallback, useEffect, useMemo, useState } from 'react';
import { X } from 'lucide-react';

import EsquemaDiagrama from './EsquemaDiagrama';
import LoadNotice from './ui/LoadNotice';
import { useLoadStatus } from './ui/load-status';
import Select, { type SelectOption } from './ui/Select';
import {
  api,
  type EsquemaDaBaseRecord,
  type PostgresBaseRecord,
  type PostgresInstanciaRecord,
} from '../lib/api';
import { formatDateTime, relativeTime } from '../lib/format';
import { useSiteScope } from './ui/site-scope-context';
import { POLL } from '../lib/polling';

const TRAVESSAO = '—';

const TODOS = '';

const UNIDADES_DE_BYTE = ['B', 'KB', 'MB', 'GB', 'TB'];

const naoMedido = (valor: number | null | undefined): boolean => valor === null || valor === undefined;

const classeDoNumero = (valor: number | null | undefined): string =>
  naoMedido(valor) ? 'text-text-faint' : 'text-text-hi';

const formatarBytes = (bytes: number | null | undefined): string => {
  if (naoMedido(bytes)) return TRAVESSAO;
  const valor = bytes as number;
  if (valor <= 0) return '0 B';
  const escala = Math.min(Math.floor(Math.log(valor) / Math.log(1024)), UNIDADES_DE_BYTE.length - 1);
  const numero = valor / 1024 ** escala;
  return `${numero.toLocaleString('pt-BR', { maximumFractionDigits: 1 })} ${UNIDADES_DE_BYTE[escala]}`;
};

const formatarInteiro = (valor: number | null | undefined): string =>
  naoMedido(valor) ? TRAVESSAO : String(valor);

const formatarTexto = (valor: string | null | undefined): string =>
  valor === null || valor === undefined || valor === '' ? TRAVESSAO : valor;

interface Marca {
  texto: string;
  classe: string;
  titulo: string;
}

const MARCA_DE_PAPEL: Record<string, Marca> = {
  primario: { texto: 'Primário', classe: 'badge-ok', titulo: 'Aceita escrita' },
  replica: { texto: 'Réplica', classe: 'badge-info', titulo: 'Em recuperação, somente leitura' },
  desconhecido: { texto: 'Desconhecido', classe: 'badge-muted', titulo: 'A sonda não conseguiu determinar o papel' },
};

const MARCA_DE_ESTADO: Record<string, Marca> = {
  ativo: { texto: 'Ativo', classe: 'badge-ok', titulo: 'Respondeu à consulta' },
  inativo: { texto: 'Inativo', classe: 'badge-crit', titulo: 'Porta detectada, mas o processo não responde' },
  sem_acesso: { texto: 'Sem acesso', classe: 'badge-warn', titulo: 'Detectado, porém sem credencial ou sem permissão' },
  desconhecido: { texto: 'Desconhecido', classe: 'badge-muted', titulo: 'A sonda não conseguiu determinar o estado' },
};

const marcaDoPapel = (papel: string): Marca => MARCA_DE_PAPEL[papel] ?? MARCA_DE_PAPEL.desconhecido;

const marcaDoEstado = (estado: string): Marca => MARCA_DE_ESTADO[estado] ?? MARCA_DE_ESTADO.desconhecido;

const NOME_DO_MOTOR: Record<string, string> = {
  postgres: 'PostgreSQL',
  mysql: 'MySQL',
  mariadb: 'MariaDB',
};

const nomeDoMotor = (motor: string): string => NOME_DO_MOTOR[motor] ?? formatarTexto(motor);

const motorComVersao = (instancia: PostgresInstanciaRecord): string => {
  const nome = nomeDoMotor(instancia.motor);
  return instancia.versao === '' ? nome : `${nome} ${instancia.versao}`;
};

const MOTORES_COM_DIAGRAMA = new Set(['postgres']);

const suportaDiagrama = (motor: string): boolean => MOTORES_COM_DIAGRAMA.has(motor);

const motivoSemDiagrama = (motor: string): string =>
  `O diagrama ainda não está disponível para ${nomeDoMotor(motor)}; o inventário mostra só as bases.`;

const enderecoDe = (instancia: PostgresInstanciaRecord): string =>
  instancia.em_container && instancia.container_nome !== ''
    ? `${instancia.container_nome} · :${instancia.porta}`
    : `${instancia.servidor_nome} · :${instancia.porta}`;

const identificacaoDe = (instancia: PostgresInstanciaRecord): string =>
  `${instancia.servidor_nome}:${instancia.porta}`;

interface ParDeValor {
  rotulo: string;
  valor: string;
}

interface LeitorDeConfiguracao {
  rotulo: string;
  ler: (instancia: PostgresInstanciaRecord) => string | number | null | undefined;
}

const CONFIGURACAO_DO_MOTOR: Record<string, LeitorDeConfiguracao[]> = {
  postgres: [
    { rotulo: 'wal_level', ler: (instancia) => instancia.wal_level },
    { rotulo: 'max_wal_senders', ler: (instancia) => instancia.max_wal_senders },
    { rotulo: 'archive_mode', ler: (instancia) => instancia.archive_mode },
  ],
};

const paresDeConfiguracao = (instancia: PostgresInstanciaRecord): ParDeValor[] =>
  (CONFIGURACAO_DO_MOTOR[instancia.motor] ?? [])
    .map(({ rotulo, ler }) => ({ rotulo, valor: ler(instancia) }))
    .filter((par) => par.valor !== null && par.valor !== undefined && par.valor !== '')
    .map((par) => ({ rotulo: par.rotulo, valor: String(par.valor) }));

const paresDeIdentificacao = (instancia: PostgresInstanciaRecord): ParDeValor[] => [
  { rotulo: 'Servidor', valor: instancia.servidor_nome },
  { rotulo: 'Endereço', valor: enderecoDe(instancia) },
  { rotulo: 'Porta', valor: String(instancia.porta) },
  {
    rotulo: 'Container',
    valor: instancia.em_container ? formatarTexto(instancia.container_nome) : 'Fora de container',
  },
  { rotulo: 'Coletado', valor: relativeTime(instancia.observado_em || null) },
];

interface Resumo {
  instancias: number;
  motores: number;
  tamanho: number | null;
  foraDeAtivo: number;
}

const resumir = (lista: PostgresInstanciaRecord[]): Resumo => {
  const medidos = lista
    .map((instancia) => instancia.tamanho_total_bytes)
    .filter((valor): valor is number => !naoMedido(valor));
  return {
    instancias: lista.length,
    motores: new Set(lista.map((instancia) => instancia.motor).filter((motor) => motor !== '')).size,
    tamanho: medidos.length === 0 ? null : medidos.reduce((soma, valor) => soma + valor, 0),
    foraDeAtivo: lista.filter((instancia) => instancia.estado !== 'ativo').length,
  };
};

const opcoesDe = (valores: string[], rotulo: (valor: string) => string): SelectOption[] => [
  { value: TODOS, label: 'Todos' },
  ...[...new Set(valores.filter((valor) => valor !== ''))]
    .sort((a, b) => a.localeCompare(b, 'pt-BR'))
    .map((valor) => ({ value: valor, label: rotulo(valor) })),
];

export interface AlvoDeEsquema {
  instanciaId: number;
  base: string;
  motor: string;
}

const ListaDePares = ({ pares, testid }: { pares: ParDeValor[]; testid: string }) => (
  <dl data-testid={testid} className="grid grid-cols-2 gap-x-6 gap-y-3">
    {pares.map((par) => (
      <div key={par.rotulo} className="flex min-w-0 flex-col gap-0.5">
        <dt className="eyebrow">{par.rotulo}</dt>
        <dd className="mono-data truncate text-sm text-text-hi" title={par.valor}>{par.valor}</dd>
      </div>
    ))}
  </dl>
);

const CartaoDaBase = ({ base }: { base: PostgresBaseRecord }) => (
  <div className="flex flex-col gap-1.5">
    <span className="mono-data text-sm text-text-hi" data-testid="base-nome">{base.nome}</span>
    <div className="flex flex-wrap gap-x-5 gap-y-1 text-xs text-text-mut">
      <span>
        Dono <span className="mono-data text-text-hi" data-testid="base-dono">{formatarTexto(base.dono)}</span>
      </span>
      <span>
        Encoding{' '}
        <span className="mono-data text-text-hi" data-testid="base-encoding">{formatarTexto(base.encoding)}</span>
      </span>
      <span>
        Tamanho{' '}
        <span className={`mono-data ${classeDoNumero(base.tamanho_bytes)}`} data-testid="base-tamanho">
          {formatarBytes(base.tamanho_bytes)}
        </span>
      </span>
      <span>
        Conexões{' '}
        <span className={`mono-data ${classeDoNumero(base.conexoes)}`} data-testid="base-conexoes">
          {formatarInteiro(base.conexoes)}
        </span>
      </span>
    </div>
  </div>
);

const PainelDeEsquema = ({ alvo, aoFechar }: { alvo: AlvoDeEsquema; aoFechar: () => void }) => {
  const [esquema, setEsquema] = useState<EsquemaDaBaseRecord | null>(null);
  const carga = useLoadStatus();
  const { ok: cargaOk, fail: cargaFail } = carga;

  useEffect(() => {
    const controller = new AbortController();
    api
      .esquemaDaBase(alvo.instanciaId, alvo.base, controller.signal)
      .then((dados) => {
        setEsquema(dados);
        cargaOk();
      })
      .catch((err) => {
        if (!controller.signal.aborted) cargaFail(err, `Falha ao ler o esquema de ${alvo.base}.`);
      });
    return () => controller.abort();
  }, [alvo.instanciaId, alvo.base, cargaOk, cargaFail]);

  return (
    <div className="fixed inset-0 z-[60] flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm" onClick={aoFechar}>
      <div
        role="dialog"
        aria-modal="true"
        aria-label={`Esquema da base ${alvo.base}`}
        data-testid="painel-esquema"
        onClick={(evento) => evento.stopPropagation()}
        className="max-h-[90vh] w-full max-w-5xl overflow-y-auto custom-scrollbar rounded-card border border-line bg-ink-900 p-5 shadow-pop"
      >
        <LoadNotice error={carga.error} className="mb-4" />
        {esquema === null ? (
          <div className="flex items-start justify-between gap-3">
            {carga.error === null ? (
              <p className="text-sm text-text-mut" data-testid="esquema-carregando">
                Lendo o esquema de {alvo.base} no servidor...
              </p>
            ) : (
              <p className="text-sm text-text-mut">
                O esquema de {alvo.base} não pôde ser lido, então não há diagrama para desenhar.
              </p>
            )}
            <button type="button" onClick={aoFechar} className="btn btn-ghost btn-sm shrink-0">
              <X size={14} strokeWidth={1.75} />
              Fechar
            </button>
          </div>
        ) : (
          <EsquemaDiagrama esquema={esquema} aoFechar={aoFechar} />
        )}
      </div>
    </div>
  );
};

const BancosView = () => {
  const { numericSiteId } = useSiteScope();
  const [instancias, setInstancias] = useState<PostgresInstanciaRecord[]>([]);
  const [servidor, setServidor] = useState(TODOS);
  const [motor, setMotor] = useState(TODOS);
  const [estado, setEstado] = useState(TODOS);
  const [selecionada, setSelecionada] = useState<number | null>(null);
  const [alvo, setAlvo] = useState<AlvoDeEsquema | null>(null);
  const [loading, setLoading] = useState(true);
  const carga = useLoadStatus();
  const { ok: cargaOk, fail: cargaFail } = carga;

  const buscar = useCallback(async (signal?: AbortSignal) => {
    try {
      setInstancias(await api.bancos(numericSiteId, signal));
      cargaOk();
    } catch (err) {
      if (!signal?.aborted) cargaFail(err, 'Falha ao listar as instâncias de banco.');
    } finally {
      setLoading(false);
    }
  }, [numericSiteId, cargaOk, cargaFail]);

  useEffect(() => {
    const controller = new AbortController();
    buscar(controller.signal);
    const timer = setInterval(() => buscar(controller.signal), POLL.bancos);
    return () => {
      clearInterval(timer);
      controller.abort();
    };
  }, [buscar]);

  const visiveis = useMemo(
    () =>
      instancias.filter(
        (instancia) =>
          (servidor === TODOS || instancia.servidor_nome === servidor) &&
          (motor === TODOS || instancia.motor === motor) &&
          (estado === TODOS || instancia.estado === estado),
      ),
    [instancias, servidor, motor, estado],
  );

  const resumo = useMemo(() => resumir(visiveis), [visiveis]);

  const detalhe = useMemo(
    () => visiveis.find((instancia) => instancia.id === selecionada) ?? null,
    [visiveis, selecionada],
  );

  const fecharDetalhe = useCallback(() => setSelecionada(null), []);

  const fecharEsquema = useCallback(() => setAlvo(null), []);

  useEffect(() => {
    if (detalhe === null && alvo === null) return;
    const aoTeclar = (evento: KeyboardEvent) => {
      if (evento.key !== 'Escape') return;
      if (alvo !== null) {
        setAlvo(null);
        return;
      }
      setSelecionada(null);
    };
    document.addEventListener('keydown', aoTeclar);
    return () => document.removeEventListener('keydown', aoTeclar);
  }, [detalhe, alvo]);

  const abrirEsquema = (instancia: PostgresInstanciaRecord, base: string) =>
    setAlvo({ instanciaId: instancia.id, base, motor: instancia.motor });

  const bases = detalhe?.bases ?? [];
  const configuracao = detalhe === null ? [] : paresDeConfiguracao(detalhe);
  const diagramaDisponivel = detalhe !== null && suportaDiagrama(detalhe.motor);

  return (
    <div className="p-4 md:p-8 anim-rise">
      <LoadNotice error={carga.error} lastOk={carga.lastOk} className="mb-4" />
      <div className="page-header flex-col items-start md:flex-row md:items-end">
        <div>
          <h1 className="page-title">Bancos de Dados</h1>
          <p className="page-desc">
            Instâncias de banco descobertas nos servidores monitorados. Abra uma linha para ver a
            identificação, a configuração do motor e as bases, e clique numa base para ver o esquema.
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-3">
          <div className="flex items-center gap-2">
            <label htmlFor="banco-servidor" className="eyebrow">Servidor</label>
            <Select
              id="banco-servidor"
              ariaLabel="Servidor"
              value={servidor}
              onChange={setServidor}
              options={opcoesDe(instancias.map((instancia) => instancia.servidor_nome), (valor) => valor)}
            />
          </div>
          <div className="flex items-center gap-2">
            <label htmlFor="banco-motor" className="eyebrow">Motor</label>
            <Select
              id="banco-motor"
              ariaLabel="Motor"
              value={motor}
              onChange={setMotor}
              options={opcoesDe(instancias.map((instancia) => instancia.motor), nomeDoMotor)}
            />
          </div>
          <div className="flex items-center gap-2">
            <label htmlFor="banco-estado" className="eyebrow">Estado</label>
            <Select
              id="banco-estado"
              ariaLabel="Estado"
              value={estado}
              onChange={setEstado}
              options={opcoesDe(
                instancias.map((instancia) => instancia.estado),
                (valor) => marcaDoEstado(valor).texto,
              )}
            />
          </div>
        </div>
      </div>

      <div className="mb-6 grid grid-cols-2 gap-3 stagger md:grid-cols-4" data-testid="resumo">
        <div className="stat-card">
          <div className="stat-value" data-testid="resumo-instancias">{resumo.instancias}</div>
          <div className="eyebrow mt-1.5">Instâncias</div>
        </div>
        <div className="stat-card">
          <div className="stat-value" data-testid="resumo-motores">{resumo.motores}</div>
          <div className="eyebrow mt-1.5">Motores distintos</div>
        </div>
        <div className="stat-card">
          <div className={`stat-value ${classeDoNumero(resumo.tamanho)}`} data-testid="resumo-tamanho">
            {formatarBytes(resumo.tamanho)}
          </div>
          <div className="eyebrow mt-1.5">Tamanho somado</div>
        </div>
        <div className="stat-card">
          <div
            className={`stat-value ${resumo.foraDeAtivo > 0 ? 'text-warn' : ''}`}
            data-testid="resumo-fora-de-ativo"
          >
            {resumo.foraDeAtivo}
          </div>
          <div className="eyebrow mt-1.5">Fora de ativo</div>
        </div>
      </div>

      <div className="panel p-6">
        <h2 className="eyebrow mb-6">Instâncias descobertas</h2>
        {loading ? (
          <p className="text-sm text-text-mut">Carregando...</p>
        ) : (
          <div className="overflow-x-auto custom-scrollbar">
            <table className="table-base min-w-[680px]">
              <thead>
                <tr>
                  <th>Motor</th>
                  <th>Instância</th>
                  <th>Papel</th>
                  <th>Estado</th>
                  <th className="text-right">Tamanho</th>
                </tr>
              </thead>
              <tbody>
                {visiveis.length === 0 ? (
                  carga.error && !carga.lastOk ? null : (
                    <tr>
                      <td colSpan={5} className="py-8 text-center text-text-mut">
                        {instancias.length === 0
                          ? 'Nenhuma instância de banco descoberta.'
                          : 'Nenhuma instância corresponde aos filtros.'}
                      </td>
                    </tr>
                  )
                ) : (
                  visiveis.map((instancia) => {
                    const papel = marcaDoPapel(instancia.papel);
                    const marca = marcaDoEstado(instancia.estado);
                    const aberta = detalhe?.id === instancia.id;

                    return (
                      <tr
                        key={instancia.id}
                        data-testid="instancia"
                        role="button"
                        tabIndex={0}
                        aria-haspopup="dialog"
                        aria-expanded={aberta}
                        aria-label={`Ver detalhe de ${identificacaoDe(instancia)}`}
                        onClick={() => setSelecionada(instancia.id)}
                        onKeyDown={(evento) => {
                          if (evento.key !== 'Enter' && evento.key !== ' ') return;
                          evento.preventDefault();
                          setSelecionada(instancia.id);
                        }}
                        className={`cursor-pointer ${aberta ? 'bg-ink-850' : ''}`}
                      >
                        <td data-testid="instancia-motor">
                          <span className="font-medium text-text-hi">{motorComVersao(instancia)}</span>
                        </td>
                        <td>
                          <div className="flex flex-col">
                            <span className="font-medium text-text-hi">{instancia.servidor_nome}</span>
                            <span className="mono-data text-xs text-text-faint" data-testid="instancia-endereco">
                              {enderecoDe(instancia)}
                            </span>
                          </div>
                        </td>
                        <td data-testid="instancia-papel">
                          <span className={`badge ${papel.classe}`} title={papel.titulo}>{papel.texto}</span>
                        </td>
                        <td data-testid="instancia-estado">
                          <div className="flex flex-col items-start gap-1">
                            <span className={`badge ${marca.classe}`} title={marca.titulo}>{marca.texto}</span>
                            {instancia.estado !== 'ativo' && instancia.motivo !== '' && (
                              <span data-testid="instancia-motivo" className="whitespace-normal text-xs text-warn">
                                {instancia.motivo}
                              </span>
                            )}
                          </div>
                        </td>
                        <td className="text-right">
                          <div className="flex flex-col items-end">
                            <span
                              data-testid="instancia-tamanho"
                              className={`mono-data ${classeDoNumero(instancia.tamanho_total_bytes)}`}
                            >
                              {formatarBytes(instancia.tamanho_total_bytes)}
                            </span>
                            <span className="text-xs text-text-faint" data-testid="instancia-bases">
                              {instancia.total_bases === 1 ? '1 base' : `${instancia.total_bases} bases`}
                            </span>
                          </div>
                        </td>
                      </tr>
                    );
                  })
                )}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {detalhe !== null && (
        <div className="fixed inset-0 z-50 flex justify-end bg-black/60 backdrop-blur-sm" onClick={fecharDetalhe}>
          <aside
            role="dialog"
            aria-modal="true"
            aria-label={`Detalhe da instância ${identificacaoDe(detalhe)}`}
            data-testid="painel-detalhe"
            onClick={(evento) => evento.stopPropagation()}
            className="h-full w-full max-w-xl overflow-y-auto custom-scrollbar border-l border-line bg-ink-900 shadow-pop"
          >
            <div className="sticky top-0 flex items-start justify-between gap-4 border-b border-line bg-ink-850 p-5">
              <div className="min-w-0">
                <h2 className="truncate text-base font-semibold text-text-hi">{motorComVersao(detalhe)}</h2>
                <p className="mono-data mt-1 truncate text-xs text-text-faint">{identificacaoDe(detalhe)}</p>
              </div>
              <button
                type="button"
                onClick={fecharDetalhe}
                aria-label="Fechar detalhe"
                className="shrink-0 text-text-faint transition-colors hover:text-text-hi"
              >
                <X size={18} strokeWidth={1.75} />
              </button>
            </div>

            <section className="border-b border-line p-5">
              <h3 className="eyebrow mb-4">Identificação</h3>
              <ListaDePares pares={paresDeIdentificacao(detalhe)} testid="identificacao" />
              <p className="mt-3 text-xs text-text-faint" title={formatDateTime(detalhe.observado_em)}>
                Última coleta em {formatDateTime(detalhe.observado_em)}.
              </p>
            </section>

            <section className="border-b border-line p-5">
              <h3 className="eyebrow mb-4">Configuração</h3>
              {configuracao.length === 0 ? (
                <p className="text-sm text-text-mut" data-testid="configuracao-vazia">
                  Nenhum parâmetro de configuração coletado para {nomeDoMotor(detalhe.motor)}.
                </p>
              ) : (
                <ListaDePares pares={configuracao} testid="configuracao" />
              )}
            </section>

            <section className="p-5">
              <h3 className="eyebrow mb-4">Bases</h3>
              {bases.length === 0 ? (
                <p className="text-sm text-text-mut">
                  Nenhuma base visível ao usuário de monitoramento nesta instância.
                </p>
              ) : (
                <ul className="flex flex-col gap-2">
                  {bases.map((base) => (
                    <li key={base.nome} data-testid="base">
                      {diagramaDisponivel ? (
                        <button
                          type="button"
                          onClick={() => abrirEsquema(detalhe, base.nome)}
                          aria-label={`Ver o esquema da base ${base.nome}`}
                          className="panel panel-hover w-full rounded-ctrl p-3 text-left"
                        >
                          <CartaoDaBase base={base} />
                        </button>
                      ) : (
                        <div className="panel rounded-ctrl p-3">
                          <CartaoDaBase base={base} />
                          <p className="mt-2 text-xs text-text-faint" data-testid="base-sem-diagrama">
                            {motivoSemDiagrama(detalhe.motor)}
                          </p>
                        </div>
                      )}
                    </li>
                  ))}
                </ul>
              )}
            </section>
          </aside>
        </div>
      )}

      {alvo !== null && (
        <PainelDeEsquema key={`${alvo.instanciaId}:${alvo.base}`} alvo={alvo} aoFechar={fecharEsquema} />
      )}
    </div>
  );
};

export default BancosView;
