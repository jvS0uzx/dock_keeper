import { Globe, Construction, ArrowRight } from 'lucide-react';

const NginxView = () => {
  return (
    <div className="p-4 md:p-8 h-full flex flex-col">
      <div className="mb-8">
        <h1 className="text-2xl font-light text-white mb-2 flex items-center gap-3">
          <Globe className="text-[#10b981]" /> Nginx & <span className="font-bold">Tráfego</span>
        </h1>
        <p className="text-[#737373] text-sm">Controle de roteamento, virtual hosts e observabilidade de borda.</p>
      </div>

      <div className="glass-panel flex-1 rounded-xl border border-white/5 bg-white/[0.02] flex flex-col items-center justify-center text-center p-8">
        <div className="w-20 h-20 rounded-full bg-[#10b981]/10 flex items-center justify-center mb-6 border border-[#10b981]/20 shadow-[0_0_30px_rgba(16,185,129,0.15)] relative">
          <Globe className="w-8 h-8 text-[#10b981]" />
          <div className="absolute -bottom-2 -right-2 bg-[#0c0c0e] rounded-full p-1.5 border border-white/10">
            <Construction className="w-4 h-4 text-[#f59e0b]" />
          </div>
        </div>
        <h2 className="text-xl font-bold text-white mb-3">Módulo em Desenvolvimento</h2>
        <p className="text-[#737373] max-w-md mx-auto leading-relaxed mb-6">
          A visualização detalhada dos Virtual Hosts e regras do Nginx será implementada em breve. Por enquanto, as métricas de tráfego e requisições ao vivo já estão disponíveis no <strong className="text-white">Dashboard Geral</strong>.
        </p>
        <button 
          onClick={() => document.querySelector<HTMLButtonElement>('[id="dashboard"]')?.click()}
          className="flex items-center gap-2 bg-[#10b981]/10 hover:bg-[#10b981]/20 text-[#10b981] border border-[#10b981]/30 px-6 py-2.5 rounded-lg font-medium transition-all"
        >
          Ir para o Dashboard <ArrowRight size={16} />
        </button>
      </div>
    </div>
  );
};

export default NginxView;
