import { useEffect, useState } from 'react';
import { AlertTriangle } from 'lucide-react';

import { api, type Readiness } from '../lib/api';

const INTERVALO_MS = 30000;

interface Props {
  irParaAlertas: () => void;
}

const numeros = (estado: Readiness): string[] => {
  const lista: string[] = [];
  if (estado.alertas_detalhe) lista.push(estado.alertas_detalhe);
  if ((estado.alertas_falhos ?? 0) > 0) lista.push(`${estado.alertas_falhos} alerta(s) sem entrega`);
  if ((estado.logs_descartados ?? 0) > 0) lista.push(`${estado.logs_descartados} linha(s) de log descartada(s)`);
  if (estado.db && estado.db !== 'ok') lista.push(`banco: ${estado.db}`);
  return lista;
};

const DegradacaoAviso = ({ irParaAlertas }: Props) => {
  const [estado, setEstado] = useState<Readiness | null>(null);

  useEffect(() => {
    let vivo = true;
    const ler = async () => {
      try {
        const atual = await api.readiness();
        if (vivo) setEstado(atual);
      } catch {
        if (vivo) setEstado(null);
      }
    };
    ler();
    const timer = setInterval(ler, INTERVALO_MS);
    return () => {
      vivo = false;
      clearInterval(timer);
    };
  }, []);

  const motivos = estado?.degradado ?? [];
  if (!estado || motivos.length === 0) return null;

  const detalhes = numeros(estado);
  const temEntregaFalha = (estado.alertas_falhos ?? 0) > 0;

  return (
    <div
      role="status"
      className="flex flex-wrap items-center gap-2 border-b border-warn/30 bg-warn/10 px-4 py-2 text-xs text-warn"
    >
      <AlertTriangle size={14} strokeWidth={1.75} className="shrink-0" />
      <span className="font-medium">Painel degradado: {motivos.join(' · ')}.</span>
      {detalhes.length > 0 && <span className="text-warn/90">{detalhes.join(' · ')}</span>}
      {temEntregaFalha && (
        <button onClick={irParaAlertas} className="ml-auto underline underline-offset-2 hover:text-warn/80">
          Ver alertas
        </button>
      )}
    </div>
  );
};

export default DegradacaoAviso;
