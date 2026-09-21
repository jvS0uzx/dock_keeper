import { useCallback, useEffect, useState } from 'react';
import { ShieldCheck, X } from 'lucide-react';

import { api, apiErrorMessage, type ServerLiveStat } from '../lib/api';
import { hasGlobalAdmin } from '../lib/panels';
import { useDialog } from './ui/dialog-context';
import { useSession } from './ui/session-context';
import LoadNotice from './ui/LoadNotice';

const MachineAdminPanel = ({ machine }: { machine: ServerLiveStat }) => {
  const dialog = useDialog();
  const { accesses } = useSession();
  const podeAdministrar = hasGlobalAdmin(accesses);
  const ehEstacao = machine.kind === 'agent';

  const [aliases, setAliases] = useState<string[] | null>(null);
  const [avisarAusencia, setAvisarAusencia] = useState(machine.absence_alert === true);
  const [erro, setErro] = useState<string | null>(null);
  const [salvando, setSalvando] = useState(false);

  const carregar = useCallback(async () => {
    try {
      const cadastro = (await api.servers()).find((s) => s.id === machine.id);
      setAliases(cadastro?.aliases ?? []);
      if (cadastro?.absence_alert !== undefined) setAvisarAusencia(cadastro.absence_alert);
      setErro(null);
    } catch (err) {
      setErro(apiErrorMessage(err, 'Falha ao ler o cadastro da máquina.'));
    }
  }, [machine.id]);

  useEffect(() => {
    if (podeAdministrar) carregar();
  }, [podeAdministrar, carregar]);

  if (!podeAdministrar) return null;

  const alternarAusencia = async (valor: boolean) => {
    setSalvando(true);
    try {
      const salvo = await api.setServerAbsenceAlert(machine.id, valor);
      setAvisarAusencia(salvo.absence_alert ?? valor);
    } catch (err) {
      dialog.notify(apiErrorMessage(err, 'Falha ao mudar o aviso de ausência.'), 'error');
    } finally {
      setSalvando(false);
    }
  };

  const removerAlias = async (alias: string) => {
    const confirmado = await dialog.confirm({
      title: `Remover ${alias}?`,
      message: `O tráfego de ${alias} deixa de ser atribuído a ${machine.name}.`,
      confirmLabel: 'Remover',
      danger: true,
    });
    if (!confirmado) return;

    const restantes = (aliases ?? []).filter((a) => a !== alias);
    setSalvando(true);
    try {
      const salvo = await api.updateServerAliases(machine.id, restantes);
      setAliases(salvo.aliases ?? restantes);
    } catch (err) {
      dialog.notify(apiErrorMessage(err, 'Falha ao remover o endereço.'), 'error');
    } finally {
      setSalvando(false);
    }
  };

  return (
    <div className="panel overflow-hidden">
      <div className="flex items-center gap-2 px-4 py-3 border-b border-line">
        <ShieldCheck size={16} strokeWidth={1.75} className="text-accent" />
        <h2 className="eyebrow">Administração</h2>
      </div>
      <div className="p-4 flex flex-col gap-5">
        {ehEstacao && (
          <div>
            <label className="inline-flex items-center gap-2 text-sm text-text cursor-pointer">
              <input
                type="checkbox"
                checked={avisarAusencia}
                disabled={salvando}
                onChange={(e) => alternarAusencia(e.target.checked)}
              />
              Avisar quando esta estação parar de enviar
            </label>
            <p className="mt-1 max-w-prose text-xs text-text-faint">
              Desligado por padrão, porque estação desligada fora do expediente geraria aviso toda noite.
            </p>
          </div>
        )}

        <div>
          <div className="eyebrow mb-2">Endereços associados à mão</div>
          {erro ? (
            <LoadNotice error={erro} />
          ) : aliases === null ? (
            <p className="text-xs text-text-faint">Carregando...</p>
          ) : aliases.length === 0 ? (
            <p className="text-xs text-text-faint">Nenhum endereço associado à mão.</p>
          ) : (
            <ul className="flex flex-wrap gap-2">
              {aliases.map((alias) => (
                <li
                  key={alias}
                  className="inline-flex items-center gap-1.5 rounded-ctrl border border-line bg-ink-850 py-1 pl-2.5 pr-1 text-xs"
                >
                  <span className="mono-data text-text selectable">{alias}</span>
                  <button
                    type="button"
                    aria-label={`Remover ${alias}`}
                    disabled={salvando}
                    onClick={() => removerAlias(alias)}
                    className="rounded p-0.5 text-text-faint transition-colors hover:text-crit"
                  >
                    <X size={13} strokeWidth={1.75} />
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>
    </div>
  );
};

export default MachineAdminPanel;
