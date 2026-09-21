import { api, type MetricaDoCatalogo } from './api';

let emCurso: Promise<MetricaDoCatalogo[]> | null = null;

export const carregarCatalogo = (): Promise<MetricaDoCatalogo[]> => {
  if (emCurso === null) {
    const pedido = Promise.resolve()
      .then(() => api.metricsCatalog())
      .catch((err) => {
        if (emCurso === pedido) emCurso = null;
        throw err;
      });
    emCurso = pedido;
  }
  return emCurso;
};

export const esquecerCatalogo = () => {
  emCurso = null;
};
