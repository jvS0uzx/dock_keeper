import { useEffect, useRef, useState } from 'react';

import { openStream } from '../../lib/api';

const ESPERA_BASE_MS = 1000;
const ESPERA_TETO_MS = 30000;
const JITTER = 0.2;

export const esperaDoBackoff = (tentativa: number, sorteio: () => number = Math.random): number => {
  const nominal = Math.min(ESPERA_BASE_MS * 2 ** tentativa, ESPERA_TETO_MS);
  return Math.round(nominal * (1 + JITTER * (sorteio() * 2 - 1)));
};

export interface StreamReconectavel {
  conectado: boolean;
  reconectando: boolean;
  erro: string | null;
}

interface Opcoes {
  path: string;
  params: Record<string, string> | null;
  onMessage: (data: string) => void;
  onAbrir?: () => void;
}

export const useStreamComReconexao = ({ path, params, onMessage, onAbrir }: Opcoes): StreamReconectavel => {
  const [estado, setEstado] = useState<StreamReconectavel>({ conectado: false, reconectando: false, erro: null });
  const mensagem = useRef(onMessage);
  const abrir = useRef(onAbrir);
  mensagem.current = onMessage;
  abrir.current = onAbrir;

  const chave = params === null ? '' : JSON.stringify([path, params]);

  useEffect(() => {
    if (!chave) {
      setEstado({ conectado: false, reconectando: false, erro: null });
      return;
    }

    const [rota, consulta] = JSON.parse(chave) as [string, Record<string, string>];
    let cancelado = false;
    let fonte: EventSource | null = null;
    let timer: ReturnType<typeof setTimeout> | null = null;
    let tentativa = 0;

    const agendar = () => {
      if (cancelado) return;
      setEstado({ conectado: false, reconectando: true, erro: null });
      timer = setTimeout(conectar, esperaDoBackoff(tentativa));
      tentativa += 1;
    };

    const conectar = () => {
      if (cancelado) return;
      openStream(rota, consulta)
        .then((es) => {
          if (cancelado) {
            es.close();
            return;
          }
          fonte = es;
          tentativa = 0;
          setEstado({ conectado: true, reconectando: false, erro: null });
          abrir.current?.();
          es.onmessage = (evento: MessageEvent) => mensagem.current(evento.data);
          es.onerror = () => {
            es.close();
            if (fonte === es) fonte = null;
            agendar();
          };
        })
        .catch((err: unknown) => {
          if (cancelado) return;
          setEstado({
            conectado: false,
            reconectando: true,
            erro: err instanceof Error ? err.message : null,
          });
          timer = setTimeout(conectar, esperaDoBackoff(tentativa));
          tentativa += 1;
        });
    };

    conectar();

    return () => {
      cancelado = true;
      if (timer) clearTimeout(timer);
      fonte?.close();
      setEstado({ conectado: false, reconectando: false, erro: null });
    };
  }, [chave]);

  return estado;
};
