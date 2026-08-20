import { useState } from 'react';
import Sidebar from './components/Sidebar';
import Dashboard from './components/Dashboard';
import ContainersView from './components/ContainersView';
import NginxView from './components/NginxView';
import SslView from './components/SslView';
import SecurityView from './components/SecurityView';
import ServersView from './components/ServersView';

function App() {
  const [activeTab, setActiveTab] = useState('dashboard');

  const renderContent = () => {
    switch (activeTab) {
      case 'dashboard':
        return <Dashboard />;
      case 'containers':
        return <ContainersView />;
      case 'nginx':
        return <NginxView />;
      case 'ssl':
        return <SslView />;
      case 'security':
        return <SecurityView />;
      case 'servers':
        return <ServersView />;
      default:
        return <Dashboard />;
    }
  };

  return (
    <div className="flex h-screen w-screen bg-[#050505] overflow-hidden text-white font-sans">
      <Sidebar activeTab={activeTab} setActiveTab={setActiveTab} />
      <main className="flex-1 overflow-y-auto relative bg-[#050505]">
        {renderContent()}
      </main>
    </div>
  );
}

export default App;
