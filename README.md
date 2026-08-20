# DockKeeper (Formerly VD Stats)

**DockKeeper** is an agentless, self-hosted platform for real-time observability and management of VPS environments, Docker containers, and NGINX Load Balancers. 

Built with Go (Backend) and React (Frontend), it connects to your servers exclusively via SSH. No agents to install on your target servers—just add your SSH keys and IPs, and you instantly get a beautiful, unified dashboard with live metrics, history, and container management.

## 🚀 Vision (SaaS Evolution)

DockKeeper is evolving from a simple CLI/monitoring tool (`vd_stats`) into a full-fledged local SaaS. The next major milestones include:
1. **Dynamic Server Management:** Transition from `.env`-based server loading to a complete CRUD interface in the Frontend, storing servers safely in PostgreSQL.
2. **Container Actions:** Not just monitoring, but allowing you to Restart/Stop/View Logs of containers directly from the UI.
3. **SSL/Domain Monitoring:** Automated cron jobs that check your domain's SSL validity and alert you before expiration.
4. **Alerting System:** Webhook/Telegram notifications when a server or container goes down or CPU/RAM spikes.

## 🏗️ Architecture

1. **Backend (Go Engine):**
   - **Agentless:** Establishes secure SSH connections using your local keys.
   - Executes non-blocking commands (`docker stats`, `top`, `df -h`) and streams logs.
   - Buffers real-time metrics in memory (Mutex lock) and flushes batches to PostgreSQL to prevent I/O bottlenecks.
   - Serves REST/WebSocket endpoints.

2. **Frontend (React + Vite + Tailwind):**
   - Sleek "glass-panel" UI with Recharts for time-series data.
   - Real-time gauges and metric tables.
   - Load Balancer traffic visualization.

3. **Database (PostgreSQL):**
   - Persists temporal data and configurations.

## 🛠️ How to Run (Development)

### 1. Environment Setup
Create a `.env` in the root:
```env
# Database
DATABASE_URL="host=localhost user=postgres password=root dbname=dockkeeper port=5432 sslmode=disable TimeZone=UTC"

# Initial Seeding (Soon to be migrated to UI-based management)
TARGET_VPS_IPS="10.0.0.1,10.0.0.2"
LB_IP="10.0.0.3"

# SSH Authentication
SSH_USER="root"
SSH_KEY_PATH="~/.ssh/id_rsa"
```

Frontend `.env` (`frontend/.env`):
```env
VITE_LB_IP="10.0.0.3"
```

### 2. Backend
```bash
cd backend
go mod tidy
go run cmd/vd_stats/main.go # Note: binary rename pending
```

### 3. Frontend
```bash
cd frontend
npm install
npm run dev
```

*Built for Developers, by Developers.*
