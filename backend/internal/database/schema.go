package database

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Server struct {
	ID string `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	// O índice composto com SiteID serve o upsert da ingestão do agente, que
	// passou a casar por (unidade, hostname): sem ele a busca é varredura.
	Name   string `gorm:"size:255;not null;index:idx_server_site_name,priority:2" json:"name"`
	HostIP string `gorm:"size:255;not null" json:"host_ip"`
	User   string `gorm:"size:100;default:'root'" json:"user"`
	Port   int    `gorm:"default:22" json:"port"`
	Kind   string `gorm:"size:20;default:'ssh'" json:"kind"` // "ssh" (coleta via SSH) ou "agent" (push)

	// CollectNginx liga o stream do access log do Nginx neste host. Antes isso
	// era decidido pelo nome do servidor ser "Load Balancer" — uma string mágica
	// que amarrava o projeto à convenção de nomes de quem o instalou.
	CollectNginx bool `gorm:"default:false" json:"collect_nginx"`

	// Unidade à qual o host pertence. Nulo = não classificado.
	SiteID *uint `gorm:"index;index:idx_server_site_name,priority:1" json:"site_id"`

	// Fatos reportados pelo agente no primeiro push. Estáticos o bastante para
	// não virarem série temporal.
	OS           string `gorm:"size:64" json:"os"`
	Platform     string `gorm:"size:128" json:"platform"`
	Arch         string `gorm:"size:32" json:"arch"`
	AgentVersion string `gorm:"size:32" json:"agent_version"`
	LastUser     string `gorm:"size:128" json:"last_user"`

	// Intervalo de report do agente, em segundos. É o que o próprio agente diz
	// estar usando (AGENT_INTERVAL), não uma configuração do painel — só ele
	// sabe o valor real. A janela de "online" é derivada daqui: um agente com
	// intervalo de 60 s aparecia permanentemente offline contra a janela fixa
	// de 30 s, mesmo reportando certo. Zero = desconhecido (coleta por SSH,
	// cujo ritmo é o do script remoto, ou agente antigo que não informa).
	ReportIntervalSec int `gorm:"default:0" json:"report_interval_sec"`

	// MachineID é o identificador que o próprio dispositivo gera no primeiro
	// boot e persiste em disco. É a chave estável do host: hostname muda quando
	// alguém renomeia a máquina, e a chave (unidade, hostname) parte o histórico
	// em duas séries quando isso acontece. Vazio para host cadastrado a mão e
	// para agente antigo que ainda não o envia — por isso a unicidade é parcial.
	MachineID string `gorm:"size:128;index" json:"machine_id"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// AlertRule é uma regra configurável de alerta sobre métricas de host.
type AlertRule struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	Name      string     `gorm:"size:255;not null" json:"name"`
	Target    string     `gorm:"size:64;not null;default:'*'" json:"target"` // server_id ou "*" (todos)
	Metric    string     `gorm:"size:32;not null" json:"metric"`             // cpu | mem | disk | load
	Operator  string     `gorm:"size:4;not null" json:"operator"`            // ">" ou "<"
	Threshold float64    `gorm:"not null" json:"threshold"`
	Enabled   bool       `gorm:"default:true" json:"enabled"`
	LastFired *time.Time `json:"last_fired"`

	// Severity classifica o alerta: info | warning | high | critical.
	// É ela que decide a cor no painel e se a notificação sai ou fica no log.
	Severity string `gorm:"size:16;not null;default:'warning'" json:"severity"`

	// DependsOnServerID suprime o alerta quando o host do qual esta máquina
	// depende está fora. Sem isso, um switch caindo gera um alerta por estação
	// atrás dele em vez de um só.
	DependsOnServerID *string `gorm:"type:uuid;index" json:"depends_on_server_id"`

	// TargetSiteID amarra a regra a uma unidade inteira. Sem ele o operador só
	// teria "todos" ou uma máquina específica — com várias filiais isso obriga
	// a recriar a mesma regra host a host, e a esquecer de aplicá-la em cada
	// estação nova. Exclusivo com Target: quando preenchido, Target fica "*".
	TargetSiteID *uint `gorm:"index" json:"target_site_id"`

	// ForDurationSec exige que a condição se mantenha por este tempo antes de
	// disparar. Zero mantém o comportamento antigo: uma única amostra acima do
	// limite já alerta, o que transforma qualquer pico de compilação ou backup
	// em incidente e ensina o operador a ignorar o canal.
	ForDurationSec int `gorm:"default:0" json:"for_duration_sec"`

	CreatedAt time.Time `json:"created_at"`
}

// LogEntry armazena linhas de log (auth.log / container) para histórico e busca.
type LogEntry struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ServerID  string    `gorm:"type:uuid;index:idx_logentry_srv_ts,priority:1" json:"server_id"`
	Source    string    `gorm:"size:20;index" json:"source"` // "auth" | "container"
	Container string    `gorm:"size:255;index" json:"container"`
	Line      string    `gorm:"type:text" json:"line"`
	Timestamp time.Time `gorm:"index:idx_logentry_srv_ts,priority:2,sort:desc" json:"timestamp"`
}

// User é uma pessoa que acessa o painel.
//
// Existe porque um token compartilhado não separa quem consulta de quem manda
// reiniciar container por SSH como root — e não diz quem fez o quê.
type User struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	Username     string     `gorm:"size:64;uniqueIndex;not null" json:"username"`
	PasswordHash string     `gorm:"size:255;not null" json:"-"`
	Role         string     `gorm:"size:16;not null;default:'viewer'" json:"role"`
	Active       bool       `gorm:"default:true" json:"active"`
	LastLogin    *time.Time `json:"last_login"`
	CreatedAt    time.Time  `json:"created_at"`
}

// UserSiteAccess restringe (ou concede) o alcance de um usuário por unidade.
//
// É o segundo eixo do modelo do Zabbix: User.Role diz o que a pessoa PODE
// FAZER; estas linhas dizem ONDE. SiteID nulo = acesso global com aquele
// papel. Usuário sem nenhuma linha mantém o comportamento antigo: o papel da
// conta vale para tudo.
type UserSiteAccess struct {
	ID     uint   `gorm:"primaryKey" json:"id"`
	UserID uint   `gorm:"index;not null" json:"user_id"`
	SiteID *uint  `gorm:"index" json:"site_id"`
	Role   string `gorm:"size:16;not null" json:"role"`
}

// Site é uma unidade física monitorada (matriz, filial, andar de um prédio).
// O painel não assume quantas existem: hosts e plantas se penduram aqui.
type Site struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:255;not null" json:"name"`
	Code      string    `gorm:"size:64;uniqueIndex" json:"code"` // apelido curto, ex: "mtz"
	Address   string    `gorm:"size:500" json:"address"`
	Latitude  float64   `json:"latitude"`  // reservado para mapa geográfico
	Longitude float64   `json:"longitude"` // multi-unidade (não usado ainda)
	CreatedAt time.Time `json:"created_at"`
}

// NetworkHost é um endereço visto na varredura da rede local. Guarda o
// inventário: o que a varredura descobre sozinha (hostname, MAC, portas) mais
// os campos cadastrais que localizam o equipamento fisicamente.
type NetworkHost struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// Índice simples, não único: a unicidade é (unidade, ip) e mora num índice
	// de expressão criado em migrateNetworkHostSiteIP, fora do alcance das tags
	// do GORM. IP único globalmente fazia duas filiais com a mesma faixa
	// RFC1918 disputarem a mesma linha.
	IP        string    `gorm:"size:45;index" json:"ip"`
	Hostname  string    `gorm:"size:255" json:"hostname"`
	MAC       string    `gorm:"size:32;index" json:"mac"`
	OpenPorts string    `gorm:"size:255" json:"open_ports"` // "22,3389,445"
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `gorm:"index" json:"last_seen"`

	// Inferido pela varredura a partir das portas abertas; o operador pode
	// sobrescrever pelo painel.
	DeviceType string `gorm:"size:64;index" json:"device_type"`

	// DeviceTypeLocked marca que o valor acima foi corrigido a mão. Sem essa
	// trava a varredura reinferia o tipo a cada ciclo e desfazia a correção do
	// técnico em no máximo 15 minutos.
	DeviceTypeLocked bool `gorm:"default:false" json:"device_type_locked"`

	// Cadastro preenchido pelo operador. Espelha o host inventory do Zabbix,
	// reduzido ao que se usa para achar a máquina numa planta.
	SiteID *uint `gorm:"index" json:"site_id"`

	// SiteLocked marca que a unidade foi escolhida a mão. O coletor remoto
	// sempre envia a unidade dele, então sem a trava ele revertia no ciclo
	// seguinte todo host que o operador tivesse movido.
	SiteLocked bool `gorm:"default:false" json:"site_locked"`

	Floor    string `gorm:"size:64" json:"floor"`
	Sector   string `gorm:"size:128;index" json:"sector"`
	Room     string `gorm:"size:64" json:"room"`
	Rack     string `gorm:"size:64" json:"rack"`
	AssetTag string `gorm:"size:64;index" json:"asset_tag"`
	Owner    string `gorm:"size:255" json:"owner"`
	Notes    string `gorm:"type:text" json:"notes"`
}

// networkHostSiteExpr é a coluna de unidade como o índice único do inventário a
// enxerga. O COALESCE existe porque o Postgres trata NULL como distinto de NULL
// num índice único: sem ele, host ainda sem unidade entraria em duplicata a
// cada ciclo do coletor. O sentinela 0 é seguro, Site.ID é serial e começa em 1.
//
// prefix é o apelido da tabela ("a.", "b.") quando a consulta usa mais de uma.
func networkHostSiteExpr(prefix string) string {
	return "COALESCE(" + prefix + "site_id, 0)"
}

// NetworkHostConflictTarget é o alvo do ON CONFLICT do inventário.
//
// Mora aqui, e não em cada chamador, porque precisa casar exatamente com o
// índice criado na migração: quando o alvo não corresponde a nenhum índice o
// Postgres recusa o INSERT inteiro, e são dois caminhos de gravação — o coletor
// remoto e a varredura local.
func NetworkHostConflictTarget() []clause.Column {
	return []clause.Column{
		{Name: networkHostSiteExpr(""), Raw: true},
		{Name: "ip"},
	}
}

// AdoptNetworkHostsWithoutSite entrega à unidade os hosts que já estavam no
// inventário sem nenhuma.
//
// Precisa rodar antes do upsert nos dois caminhos de gravação. O índice único é
// (COALESCE(site_id,0), ip): a linha antiga sem unidade tem chave (0, ip) e não
// colide com a nova, que tem (unidade, ip). Sem a adoção o mesmo endereço passa
// a existir duas vezes, uma órfã e uma classificada, e a varredura local perde a
// capacidade de classificar host que ainda não tinha unidade.
//
// site_locked é respeitado: unidade nula escolhida a mão continua nula. O NOT
// EXISTS evita violar o índice quando as duas linhas já existem.
func AdoptNetworkHostsWithoutSite(siteID uint, ips []string) error {
	if siteID == 0 || len(ips) == 0 {
		return nil
	}
	return DB.Exec(`
		UPDATE network_hosts AS h
		SET site_id = ?
		WHERE h.site_id IS NULL
		  AND h.site_locked = false
		  AND h.ip IN ?
		  AND NOT EXISTS (
			SELECT 1 FROM network_hosts AS o
			WHERE o.ip = h.ip AND o.site_id = ?
		  )`, siteID, ips, siteID).Error
}

// FloorPlan é uma planta baixa: imagem de fundo sobre a qual os hosts são
// posicionados. Equivale ao Map do Zabbix / Canvas panel do Grafana.
type FloorPlan struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	SiteID      *uint     `gorm:"index" json:"site_id"`
	Name        string    `gorm:"size:255;not null" json:"name"`
	ImagePath   string    `gorm:"size:500" json:"-"` // caminho em disco, nunca exposto
	ContentType string    `gorm:"size:64" json:"content_type"`
	Width       int       `json:"width"`
	Height      int       `json:"height"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// FloorPlanPin é um marcador posicionado na planta. Aponta para um host do
// inventário ou, no caso de drill-down, para outra planta.
type FloorPlanPin struct {
	ID     uint `gorm:"primaryKey" json:"id"`
	PlanID uint `gorm:"index;not null" json:"plan_id"`

	// O endereço sozinho NÃO identifica o host: a unicidade do inventário é
	// (COALESCE(site_id,0), ip), então o mesmo 192.168.0.10 existe em cada
	// filial com faixa RFC1918 repetida. A unidade que fecha a chave é a da
	// planta (FloorPlan.SiteID), e por isso não precisa ser gravada aqui — um
	// marcador só existe dentro de uma planta, e a planta é de um lugar
	// físico. Quem resolve o par é pinsWithState, em internal/api.
	HostIP string `gorm:"size:45;index" json:"host_ip"`
	Label  string `gorm:"size:255" json:"label"`

	// Coordenadas em porcentagem da imagem (0-100), não em pixel: assim o
	// marcador acompanha o redimensionamento da planta na tela.
	X float64 `json:"x"`
	Y float64 `json:"y"`

	// Quando preenchido, clicar no pin abre outra planta (prédio -> andar).
	TargetPlanID *uint `json:"target_plan_id"`
}

type Container struct {
	ID         string `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	ServerID   string `gorm:"type:uuid;index;not null"`
	DockerID   string `gorm:"size:64;not null;index"`
	Name       string `gorm:"size:255;not null"`
	ProjectDir string `gorm:"size:500"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type MetricServer struct {
	ID              uint   `gorm:"primaryKey"`
	ServerID        string `gorm:"type:uuid;not null;index:idx_metricserver_srv_ts,priority:1"`
	UptimeSeconds   float64
	DiskUsedBytes   int64
	DiskTotalBytes  int64
	CPUUsagePercent float64
	MemUsedBytes    int64
	MemTotalBytes   int64
	LoadAvg1        float64

	// SSHHandshakeMs é o tempo do handshake SSH inteiro (TCP + troca de chaves)
	// medido ao abrir a sessão de coleta — NÃO é RTT de rede, e fica uma ordem
	// de grandeza acima dele (1000-1400 ms nas VPS atuais). O campo se chamava
	// PingLatencyMs e o painel rotulava "Latência", o que induzia o operador a
	// ler o número como latência de rede. A coluna continua `ping_latency_ms`
	// de propósito: renomeá-la faria o AutoMigrate criar uma coluna nova e
	// abandonar todo o histórico já gravado.
	//
	// RTT de verdade exigiria um prober separado (ICMP ou TCP periódico), que
	// não existe no projeto hoje.
	//
	// Nulo = a fonte não mede (agente push não abre sessão SSH).
	SSHHandshakeMs *float64 `gorm:"column:ping_latency_ms"`

	// Maior temperatura reportada pelos sensores, em °C.
	//
	// Nulo = a fonte não mede ou a máquina não tem sensor (VM, container). Era
	// float64 e o "não medido" virava zero, indistinguível de uma leitura real
	// de zero: o gráfico de temperatura de uma VPS era uma reta no chão.
	//
	// Atenção à migração: o AutoMigrate não afrouxa a nulidade de uma coluna
	// que já existe, e as linhas gravadas antes desta mudança seguem com 0.
	// Só o dado novo distingue os dois casos.
	TemperatureC *float64

	Timestamp time.Time `gorm:"not null;index:idx_metricserver_srv_ts,priority:2,sort:desc"`
}

// MetricServerTrend é a agregação horária de MetricServer.
//
// Gráfico de 30 dias sobre o dado bruto varre milhões de linhas; sobre a trend
// são 24 linhas por dia por host. É o mesmo princípio dos "trends" do Zabbix:
// o histórico fino tem vida curta, a série longa vive agregada.
type MetricServerTrend struct {
	ID       uint      `gorm:"primaryKey"`
	ServerID string    `gorm:"type:uuid;not null;uniqueIndex:idx_trend_srv_bucket,priority:1"`
	Bucket   time.Time `gorm:"not null;uniqueIndex:idx_trend_srv_bucket,priority:2"`

	// Não nuláveis: as colunas de origem são NOT NULL na prática e o grupo
	// nunca é vazio, então AVG e MAX sempre devolvem número.
	CPUAvg, CPUMax           float64
	LoadAvg1Avg, LoadAvg1Max float64
	Samples                  int

	// Ponteiros porque a agregação devolve NULL de verdade, e NULL não entra
	// num float64 — o Scan erra a linha inteira. É o mesmo problema já
	// corrigido em MetricServer que não tinha sido propagado para a trend.
	//
	// Percentual de memória e de disco: o NULLIF do divisor anula a amostra de
	// host que reportou total zero; hora inteira assim resulta em NULL.
	// Temperatura: nula quando nenhuma amostra da hora tinha sensor.
	//
	// Nulo é "não medido" e precisa continuar distinguível de zero — zero é uma
	// leitura como qualquer outra na tela.
	MemPercentAvg  *float64
	DiskPercentAvg *float64
	TemperatureAvg *float64
	TemperatureMax *float64
}

type MetricContainer struct {
	ID              uint      `gorm:"primaryKey"`
	ContainerID     string    `gorm:"type:uuid;not null;index:idx_metriccontainer_ct_ts,priority:1"`
	CPUUsagePercent float64   `gorm:"not null"`
	MemUsedBytes    int64     `gorm:"not null"`
	MemLimitBytes   int64     `gorm:"not null"`
	State           string    `gorm:"size:50;not null;default:'running'"`
	Status          string    `gorm:"size:255;not null;default:''"`
	Timestamp       time.Time `gorm:"not null;index:idx_metriccontainer_ct_ts,priority:2,sort:desc"`
}

type MetricLoadBalancer struct {
	ID           uint   `gorm:"primaryKey"`
	UpstreamAddr string `gorm:"size:255;not null"`
	ServerName   string `gorm:"size:255"`
	Status       string `gorm:"size:10"`

	// ServerID e SiteID amarram a linha ao host que produziu o access log.
	// Sem eles a tabela não tinha unidade nenhuma, e a rota de descoberta de SSL
	// só sabia devolver a topologia inteira ou lista vazia — segurança comprada
	// com uma tela quebrada para quem tem papel só numa filial.
	ServerID *string `gorm:"type:uuid;index" json:"server_id"`
	SiteID   *uint   `gorm:"index" json:"site_id"`

	RequestsCount int       `gorm:"not null"`
	Timestamp     time.Time `gorm:"index;not null"`
}

type Domain struct {
	ID   uint   `gorm:"primaryKey" json:"id"`
	Name string `gorm:"size:255;not null;unique" json:"domain"`
	// Ponteiro porque a coluna é uuid e o vínculo com servidor é opcional:
	// gravar string vazia num uuid faz o Postgres recusar a linha inteira.
	ServerID  *string   `gorm:"type:uuid;index" json:"server_id"`
	CreatedAt time.Time `json:"created_at"`

	// Estado do certificado, preenchido pelo worker de SSL (não pelo usuário).
	Valid    bool   `gorm:"default:false" json:"valid"`
	Issuer   string `gorm:"size:255" json:"issuer"`
	DaysLeft int    `gorm:"default:0" json:"days_left"`
	ErrorMsg string `gorm:"size:500" json:"error_msg"`

	// InvalidReason é o motivo em forma legível por máquina: expirado,
	// ainda_nao_valido, hostname_divergente, autoassinado, cadeia_nao_confiavel,
	// sem_certificado, handshake. ErrorMsg continua carregando TODOS os
	// problemas em texto; esta coluna guarda só o mais grave, que é o que a tela
	// precisa para escolher um rótulo em vez de imprimir uma frase.
	InvalidReason string `gorm:"size:40;index" json:"invalid_reason"`

	LastCheck *time.Time `json:"last_check"`
}
