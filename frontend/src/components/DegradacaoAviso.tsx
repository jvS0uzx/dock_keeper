import { useEffect, useState } from 'react';
import { AlertTriangle, EyeOff } from 'lucide-react';

import { api, apiErrorMessage, type Readiness } from '../lib/api';
import { POLL } from '../lib/polling';


interface Props {
  irParaAlertas: () => void;
}

const numeros = (estado: Readiness): string[] => {
  const lista: string[] = [];
  if (estado.alertas_detalhe) lista.push(estado.alertas_detalhe);
  if ((estado.alertas_falhos ?? 0) > 0) lista.push(`${estado.alertas_falhos} alerta(s) sem entrega`);
  if ((estado.alertas_sem_canal ?? 0) > 0)
    lista.push(`${estado.alertas_sem_canal} alerta(s) sem canal de entrega configurado`);
  if ((estado.logs_descartados ?? 0) > 0) lista.push(`${estado.logs_descartados} linha(s) de log descartada(s)`);
  if (estado.db && estado.db !== 'ok') lista.push(`banco: ${estado.db}`);
  return lista;
};

const DegradacaoAviso = ({ irParaAlertas }: Props) => {
  const [estado, setEstado] = useState<Readiness | null>(null);
  const [erro, setErro] = useState<string | null>(null);

  useEffect(() => {
    let vivo = true;
    const ler = async () => {
      try {
        const atual = await api.readiness();
        if (vivo) {
          setEstado(atual);
          setErro(null);
        }
      } catch (err) {
        if (vivo) {
          setEstado(null);
          setErro(apiErrorMessage(err, 'a rota de prontidão não respondeu'));
        }
      }
    };
    ler();
    const timer = setInterval(ler, POLL.saudeDoPainel);
    return () => {
      vivo = false;
      clearInterval(timer);
    };
  }, []);

  if (erro) {
    return (
      <div
        role="status"
        className="flex flex-wrap items-center gap-2 border-b border-line bg-ink-850 px-4 py-2 text-xs text-text-mut"
      >
        <EyeOff size={14} strokeWidth={1.75} className="shrink-0" />
        <span className="font-medium text-text">Prontidão do painel sem leitura: {erro}.</span>
        <span>A saúde do painel não está sendo medida. Nova tentativa a cada 30 s.</span>
      </div>
    );
  }

  const motivos = estado?.degradado ?? [];
  if (!estado || motivos.length === 0) return null;

  const detalhes = numeros(estado);
  const temEntregaParada = (estado.alertas_falhos ?? 0) > 0 || (estado.alertas_sem_canal ?? 0) > 0;

  return (
    <div
      role="status"
      className="flex flex-wrap items-center gap-2 border-b border-warn/30 bg-warn/10 px-4 py-2 text-xs text-warn"
    >
      <AlertTriangle size={14} strokeWidth={1.75} className="shrink-0" />
      <span className="font-medium">Painel degradado: {motivos.join(' · ')}.</span>
      {detalhes.length > 0 && <span className="text-warn/90">{detalhes.join(' · ')}</span>}
      {temEntregaParada && (
        <button onClick={irParaAlertas} className="ml-auto font-medium text-warn transition-colors hover:text-accent-hi">
          Ver alertas
        </button>
      )}
    </div>
  );
};

export default DegradacaoAviso;
