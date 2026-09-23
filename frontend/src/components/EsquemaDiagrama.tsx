import { useId, useMemo, useState } from 'react';
import { Table2, TriangleAlert, Waypoints, X } from 'lucide-react';

import Select from './ui/Select';
import { relativeTime } from '../lib/format';
import {
  ALTURA_DA_COLUNA,
  ALTURA_DO_CABECALHO,
  SALTOS_MAXIMOS,
  SALTOS_MINIMOS,
  calcularLayout,
  idDaTabela,
  schemaPadrao,
  schemasDoEsquema,
  type ArestaDoDiagrama,
  type EsquemaDaBase,
  type NoDoDiagrama,
  type TabelaDoEsquema,
} from '../lib/erd';

const TRAVESSAO = '—';

const UNIDADES_DE_BYTE = ['B', 'KB', 'MB', 'GB', 'TB'];

const formatarBytes = (bytes: number | null | undefined): string => {
  if (bytes === null || bytes === undefined) return TRAVESSAO;
  if (bytes <= 0) return '0 B';
  const escala = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), UNIDADES_DE_BYTE.length - 1);
  return `${parseFloat((bytes / 1024 ** escala).toFixed(2))} ${UNIDADES_DE_BYTE[escala]}`;
};

const formatarInteiro = (valor: number | null | undefined): string =>
  valor === null || valor === undefined ? TRAVESSAO : String(valor);

const caminhoDaAresta = (aresta: ArestaDoDiagrama): string => {
  const [inicio, primeiro, segundo, fim] = aresta.pontos;
  return `M ${inicio.x} ${inicio.y} C ${primeiro.x} ${primeiro.y}, ${segundo.x} ${segundo.y}, ${fim.x} ${fim.y}`;
};

const ListaDeTabelas = ({ tabelas }: { tabelas: TabelaDoEsquema[] }) => (
  <table className="table-base" data-testid="lista-de-tabelas">
    <thead>
      <tr>
        <th>Tabela</th>
        <th className="text-right">Linhas estimadas</th>
        <th className="text-right">Tamanho</th>
      </tr>
    </thead>
    <tbody>
      {tabelas.map((tabela) => (
        <tr key={idDaTabela(tabela.schema, tabela.nome)} data-testid="tabela-listada">
          <td className="mono-data text-text-hi">{tabela.nome}</td>
          <td
            className={`mono-data text-right ${tabela.linhas_estimadas === null ? 'text-text-faint' : 'text-text-mut'}`}
          >
            {formatarInteiro(tabela.linhas_estimadas)}
          </td>
          <td
            className={`mono-data text-right ${tabela.tamanho_bytes === null ? 'text-text-faint' : 'text-text-mut'}`}
          >
            {formatarBytes(tabela.tamanho_bytes)}
          </td>
        </tr>
      ))}
    </tbody>
  </table>
);

const NoDaTabela = ({
  no,
  emFoco,
  aoFocar,
}: {
  no: NoDoDiagrama;
  emFoco: boolean;
  aoFocar: (id: string) => void;
}) => {
  const borda = emFoco ? 'var(--color-accent)' : 'var(--color-line-hi)';

  return (
    <g
      data-testid="no-do-diagrama"
      data-tabela={no.id}
      role="button"
      tabIndex={0}
      aria-label={`Ver a vizinhança de ${no.tabela}`}
      className="cursor-pointer"
      onClick={() => aoFocar(no.id)}
      onKeyDown={(evento) => {
        if (evento.key === 'Enter' || evento.key === ' ') {
          evento.preventDefault();
          aoFocar(no.id);
        }
      }}
    >
      <rect
        x={no.x}
        y={no.y}
        width={no.largura}
        height={no.altura}
        rx="8"
        fill="var(--color-ink-850)"
        stroke={borda}
        strokeWidth={emFoco ? 2 : 1}
      />
      <text
        x={no.x + 12}
        y={no.y + 21}
        fill="var(--color-text-hi)"
        fontSize="12"
        fontWeight="600"
        fontFamily="var(--font-mono)"
      >
        {no.tabela}
      </text>
      {no.colunas.map((coluna, indice) => {
        const y = no.y + ALTURA_DO_CABECALHO + indice * ALTURA_DA_COLUNA + 12;
        return (
          <g key={coluna.nome}>
            <text
              x={no.x + 12}
              y={y}
              fill={coluna.primaria ? 'var(--color-text-hi)' : 'var(--color-text-mut)'}
              fontSize="10"
              fontFamily="var(--font-mono)"
            >
              {coluna.nome}
            </text>
            <text
              x={no.x + no.largura - 12}
              y={y}
              textAnchor="end"
              fill={coluna.primaria ? 'var(--color-accent)' : 'var(--color-text-faint)'}
              fontSize="9"
              fontFamily="var(--font-mono)"
            >
              {coluna.primaria ? 'PK' : coluna.estrangeira ? 'FK' : ''}
            </text>
          </g>
        );
      })}
      {no.colunasOcultas > 0 && (
        <text
          x={no.x + 12}
          y={no.y + ALTURA_DO_CABECALHO + no.colunas.length * ALTURA_DA_COLUNA + 4}
          fill="var(--color-text-faint)"
          fontSize="9"
        >
          {`mais ${no.colunasOcultas} chave(s)`}
        </text>
      )}
    </g>
  );
};

const EsquemaDiagrama = ({
  esquema,
  aoFechar,
}: {
  esquema: EsquemaDaBase;
  aoFechar?: () => void;
}) => {
  const marcador = useId().replace(/[^a-zA-Z0-9]/g, '');
  const schemas = useMemo(() => schemasDoEsquema(esquema), [esquema]);
  const padrao = useMemo(() => schemaPadrao(esquema), [esquema]);
  const [escolhido, setEscolhido] = useState<string | null>(null);
  const [raiz, setRaiz] = useState('');
  const [saltos, setSaltos] = useState(SALTOS_MINIMOS);

  const schema = escolhido !== null && schemas.includes(escolhido) ? escolhido : padrao;

  const completo = useMemo(() => calcularLayout(esquema, { schema }), [esquema, schema]);
  const layout = useMemo(
    () => calcularLayout(esquema, { schema, raiz: raiz === '' ? null : raiz, saltos }),
    [esquema, schema, raiz, saltos],
  );

  const doSchema = useMemo(
    () => (esquema.tabelas ?? []).filter((tabela) => schema === null || tabela.schema === schema),
    [esquema, schema],
  );

  const semRelacaoNaBase = (esquema.relacoes ?? []).length === 0;
  const totalDesenhado = layout.nos.length;

  const aviso = esquema.truncado ? (
    <p
      data-testid="aviso-de-truncamento"
      role="status"
      className="flex items-start gap-2 rounded-ctrl border border-line bg-ink-850 p-3 text-xs text-warn"
    >
      <TriangleAlert size={14} strokeWidth={1.75} className="mt-0.5 shrink-0" />
      <span>
        {`Esta base tem ${esquema.total_tabelas} tabelas e o diagrama desenha ${totalDesenhado}, as mais conectadas. O resto ficou de fora da coleta.`}
      </span>
    </p>
  ) : null;

  const cabecalho = (
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div>
        <div className="flex items-center gap-2">
          <Waypoints size={16} strokeWidth={1.75} className="text-text-faint" />
          <h2 className="text-sm font-semibold text-text-hi">Esquema da base {esquema.base}</h2>
        </div>
        <p className="mt-1 text-xs text-text-mut">
          {`Motor ${esquema.motor} · coletado ${relativeTime(esquema.coletado_em || null)} · ${esquema.total_tabelas} tabela(s)`}
        </p>
      </div>
      {aoFechar && (
        <button type="button" className="btn btn-ghost btn-sm" onClick={aoFechar}>
          <X size={14} strokeWidth={1.75} />
          Fechar
        </button>
      )}
    </div>
  );

  if (!esquema.suporta_diagrama) {
    return (
      <div className="flex flex-col gap-4" data-testid="esquema-diagrama">
        {cabecalho}
        {aviso}
        <p
          data-testid="motor-sem-diagrama"
          className="rounded-ctrl border border-line bg-ink-850 p-3 text-xs text-text-mut"
        >
          {`O motor ${esquema.motor} não declara relacionamento entre tabelas, então não há diagrama para desenhar. Abaixo, as tabelas que a coleta encontrou.`}
        </p>
        <ListaDeTabelas tabelas={doSchema} />
      </div>
    );
  }

  if (semRelacaoNaBase) {
    return (
      <div className="flex flex-col gap-4" data-testid="esquema-diagrama">
        {cabecalho}
        {aviso}
        <p
          data-testid="sem-chave-estrangeira"
          className="rounded-ctrl border border-line bg-ink-850 p-3 text-xs text-text-mut"
        >
          Nenhuma chave estrangeira declarada nesta base. Isso é comum quando o ORM cria as
          tabelas sem constraint, e não significa falha na coleta. Abaixo, as tabelas que existem.
        </p>
        <ListaDeTabelas tabelas={doSchema} />
      </div>
    );
  }

  const opcoesDeTabela = [
    { value: '', label: 'Diagrama inteiro' },
    ...doSchema.map((tabela) => ({
      value: idDaTabela(tabela.schema, tabela.nome),
      label: tabela.nome,
    })),
  ];

  return (
    <div className="flex flex-col gap-4" data-testid="esquema-diagrama">
      {cabecalho}
      {aviso}

      <div className="flex flex-wrap items-center gap-3">
        {schemas.length > 1 && (
          <Select
            ariaLabel="Schema"
            className="w-44"
            value={schema ?? ''}
            onChange={(valor) => {
              setEscolhido(valor);
              setRaiz('');
            }}
            options={schemas.map((nome) => ({ value: nome, label: nome }))}
          />
        )}
        <Select
          ariaLabel="Tabela em foco"
          className="w-52"
          value={raiz}
          onChange={setRaiz}
          options={opcoesDeTabela}
        />
        <Select
          ariaLabel="Saltos a partir da tabela"
          className="w-36"
          value={String(saltos)}
          disabled={raiz === ''}
          onChange={(valor) => setSaltos(Number(valor) === SALTOS_MAXIMOS ? SALTOS_MAXIMOS : SALTOS_MINIMOS)}
          options={[
            { value: String(SALTOS_MINIMOS), label: '1 salto' },
            { value: String(SALTOS_MAXIMOS), label: '2 saltos' },
          ]}
        />
        <span className="text-xs text-text-mut" data-testid="contagem-desenhada">
          {`${totalDesenhado} tabela(s) no desenho · ${layout.arestas.length} relação(ões)`}
        </span>
      </div>

      <div className="flex flex-col gap-4 lg:flex-row">
        <div className="panel min-h-[280px] flex-1 overflow-auto custom-scrollbar p-2">
          {layout.nos.length === 0 ? (
            <p data-testid="schema-sem-relacao" className="p-4 text-xs text-text-mut">
              Nenhuma chave estrangeira entre as tabelas deste schema. As tabelas aparecem na lista
              ao lado.
            </p>
          ) : (
            <svg
              data-testid="tela-do-diagrama"
              viewBox={`0 0 ${layout.largura} ${layout.altura}`}
              width={layout.largura}
              height={layout.altura}
              role="img"
              aria-label={`Diagrama do esquema ${schema ?? ''} da base ${esquema.base}`}
            >
              <defs>
                <marker
                  id={`seta-${marcador}`}
                  markerWidth="8"
                  markerHeight="8"
                  refX="7"
                  refY="4"
                  orient="auto"
                >
                  <path d="M 0 0 L 8 4 L 0 8 z" fill="var(--color-text-faint)" />
                </marker>
              </defs>

              {layout.arestas.map((aresta) => (
                <g key={aresta.id} data-testid="aresta-do-diagrama" data-auto={aresta.auto}>
                  <path
                    d={caminhoDaAresta(aresta)}
                    fill="none"
                    stroke="var(--color-text-faint)"
                    strokeWidth="1.5"
                    strokeOpacity="0.55"
                    markerEnd={`url(#seta-${marcador})`}
                  />
                  <text
                    x={aresta.rotuloX}
                    y={aresta.rotuloY}
                    textAnchor="middle"
                    fill="var(--color-text-mut)"
                    fontSize="9"
                    fontFamily="var(--font-mono)"
                    stroke="var(--color-ink-900)"
                    strokeWidth="3"
                    paintOrder="stroke"
                  >
                    {aresta.rotulo}
                  </text>
                </g>
              ))}

              {layout.nos.map((no) => (
                <NoDaTabela
                  key={no.id}
                  no={no}
                  emFoco={no.id === layout.raiz}
                  aoFocar={(id) => setRaiz((atual) => (atual === id ? '' : id))}
                />
              ))}
            </svg>
          )}
        </div>

        {completo.orfas.length > 0 && (
          <aside className="panel w-full p-4 lg:w-72" data-testid="tabelas-sem-relacao">
            <div className="mb-3 flex items-center gap-2">
              <Table2 size={14} strokeWidth={1.75} className="text-text-faint" />
              <h3 className="eyebrow">Tabelas sem relação</h3>
            </div>
            <p className="mb-3 text-xs text-text-mut">
              Nenhuma chave estrangeira entra ou sai destas tabelas, então elas ficam fora do
              desenho.
            </p>
            <ul className="flex flex-col gap-2">
              {completo.orfas.map((tabela) => (
                <li
                  key={idDaTabela(tabela.schema, tabela.nome)}
                  data-testid="tabela-orfa"
                  className="flex items-center justify-between gap-2 rounded-ctrl border border-line bg-ink-850 px-3 py-2"
                >
                  <span className="mono-data text-xs text-text-hi">{tabela.nome}</span>
                  <span
                    className={`mono-data text-xs ${tabela.tamanho_bytes === null ? 'text-text-faint' : 'text-text-mut'}`}
                  >
                    {formatarBytes(tabela.tamanho_bytes)}
                  </span>
                </li>
              ))}
            </ul>
          </aside>
        )}
      </div>
    </div>
  );
};

export default EsquemaDiagrama;
