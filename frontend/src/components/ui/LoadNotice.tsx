import { AlertTriangle } from 'lucide-react';

interface LoadNoticeProps {
  error: string | null;
  lastOk?: Date | null;
  className?: string;
}

const hora = (d: Date) => d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });

const LoadNotice = ({ error, lastOk = null, className = '' }: LoadNoticeProps) => {
  if (!error) return null;
  const stale = lastOk !== null;
  return (
    <p role="alert" className={`flex items-start gap-2 text-xs ${stale ? 'text-warn' : 'text-crit'} ${className}`}>
      <AlertTriangle size={14} strokeWidth={1.75} className="shrink-0 mt-px" />
      <span>{stale ? `Dados de ${hora(lastOk)}. A última atualização falhou: ${error}` : error}</span>
    </p>
  );
};

export default LoadNotice;
