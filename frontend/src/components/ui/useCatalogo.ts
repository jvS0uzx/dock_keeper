import { useCallback, useEffect, useMemo, useState } from 'react';

import { apiErrorMessage, type MetricaDoCatalogo } from '../../lib/api';
import { carregarCatalogo } from '../../lib/catalogo';

export const useCatalogo = () => {
  const [metricas, setMetricas] = useState<MetricaDoCatalogo[]>([]);
  const [erro, setErro] = useState<string | null>(null);
  const [tentativa, setTentativa] = useState(0);

  useEffect(() => {
    let vivo = true;
    carregarCatalogo()
      .then((lista) => {
        if (!vivo) return;
        setMetricas(lista);
        setErro(null);
      })
      .catch((err) => {
        if (vivo) setErro(apiErrorMessage(err, 'Falha ao ler o catálogo de métricas.'));
      });
    return () => {
      vivo = false;
    };
  }, [tentativa]);

  const buscar = useCallback((nome: string) => metricas.find((m) => m.nome === nome), [metricas]);
  const rotulo = useCallback((nome: string) => buscar(nome)?.rotulo ?? nome, [buscar]);
  const unidade = useCallback((nome: string) => buscar(nome)?.unidade ?? '', [buscar]);
  const recarregar = useCallback(() => setTentativa((n) => n + 1), []);

  return useMemo(
    () => ({ metricas, erro, buscar, rotulo, unidade, recarregar }),
    [metricas, erro, buscar, rotulo, unidade, recarregar],
  );
};
