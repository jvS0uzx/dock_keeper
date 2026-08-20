import { ShieldAlert, Construction } from 'lucide-react';

const SecurityView = () => {
  return (
    <div className="p-4 md:p-8 h-full flex flex-col">
      <div className="mb-8">
        <h1 className="text-2xl font-light text-white mb-2 flex items-center gap-3">
          <ShieldAlert className="text-[#10b981]" /> Segurança & <span className="font-bold">Auditoria</span>
        </h1>
        <p className="text-[#737373] text-sm">Logs de acesso SSH, tentativas de intrusão, regras de Firewall (UFW) e Fail2Ban.</p>
      </div>

      <div className="glass-panel flex-1 rounded-xl border border-white/5 bg-white/[0.02] flex flex-col items-center justify-center text-center p-8">
        <div className="w-20 h-20 rounded-full bg-[#10b981]/10 flex items-center justify-center mb-6 border border-[#10b981]/20 shadow-[0_0_30px_rgba(16,185,129,0.15)] relative">
          <ShieldAlert className="w-8 h-8 text-[#10b981]" />
          <div className="absolute -bottom-2 -right-2 bg-[#0c0c0e] rounded-full p-1.5 border border-white/10">
            <Construction className="w-4 h-4 text-[#f59e0b]" />
          </div>
        </div>
        <h2 className="text-xl font-bold text-white mb-3">Módulo em Desenvolvimento</h2>
        <p className="text-[#737373] max-w-md mx-auto leading-relaxed">
          O controle centralizado de Firewall, banimento automático de IPs maliciosos e visualização de trilhas de auditoria farão parte da Fase 3 do projeto DockKeeper.
        </p>
      </div>
    </div>
  );
};

export default SecurityView;
