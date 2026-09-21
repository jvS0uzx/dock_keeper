package metricas

import "github.com/jvS0uzx/dock_keeper/internal/database"

const (
	EscopoServidor  = "servidor"
	EscopoContainer = "container"
	EscopoAmbos     = "ambos"
)

type Metrica struct {
	Nome    string
	Rotulo  string
	Unidade string
	Escopo  string

	SerieDoServidor  string
	SerieDoContainer string
	TendenciaMedia   string
	TendenciaMaxima  string

	Valor func(database.MetricServer) (float64, bool)
}

func medido(v *float64) (float64, bool) {
	if v == nil {
		return 0, false
	}
	return *v, true
}

func percentual(usado, total int64) (float64, bool) {
	if total <= 0 {
		return 0, false
	}
	return float64(usado) / float64(total) * 100, true
}

var registro = []Metrica{
	{
		Nome: "cpu", Rotulo: "CPU", Unidade: "%", Escopo: EscopoAmbos,
		SerieDoServidor: "cpu_usage_percent", SerieDoContainer: "cpu_usage_percent",
		TendenciaMedia: "cpu_avg", TendenciaMaxima: "cpu_max",
		Valor: func(m database.MetricServer) (float64, bool) { return medido(m.CPUUsagePercent) },
	},
	{
		Nome: "mem", Rotulo: "Memória", Unidade: "%", Escopo: EscopoAmbos,
		SerieDoServidor:  "mem_used_bytes::float8 / NULLIF(mem_total_bytes, 0) * 100",
		SerieDoContainer: "mem_used_bytes::float8 / NULLIF(mem_limit_bytes, 0) * 100",
		TendenciaMedia:   "mem_percent_avg",
		Valor:            func(m database.MetricServer) (float64, bool) { return percentual(m.MemUsedBytes, m.MemTotalBytes) },
	},
	{
		Nome: "disk", Rotulo: "Disco", Unidade: "%", Escopo: EscopoServidor,
		SerieDoServidor: "disk_used_bytes::float8 / NULLIF(disk_total_bytes, 0) * 100",
		TendenciaMedia:  "disk_percent_avg",
		Valor:           func(m database.MetricServer) (float64, bool) { return percentual(m.DiskUsedBytes, m.DiskTotalBytes) },
	},
	{
		Nome: "load", Rotulo: "Carga (1 min)", Unidade: "", Escopo: EscopoServidor,
		SerieDoServidor: "load_avg1", TendenciaMedia: "load_avg1_avg", TendenciaMaxima: "load_avg1_max",
		Valor: func(m database.MetricServer) (float64, bool) { return medido(m.LoadAvg1) },
	},
	{
		Nome: "temperature", Rotulo: "Temperatura", Unidade: "°C", Escopo: EscopoServidor,
		SerieDoServidor: "temperature_c", TendenciaMedia: "temperature_avg", TendenciaMaxima: "temperature_max",
		Valor: func(m database.MetricServer) (float64, bool) { return medido(m.TemperatureC) },
	},
	{
		Nome: "net_rx", Rotulo: "Rede recebida", Unidade: "bytes/s", Escopo: EscopoServidor,
		SerieDoServidor: "net_rx_bps", TendenciaMedia: "net_rx_avg",
		Valor: func(m database.MetricServer) (float64, bool) { return medido(m.NetRxBps) },
	},
	{
		Nome: "net_tx", Rotulo: "Rede enviada", Unidade: "bytes/s", Escopo: EscopoServidor,
		SerieDoServidor: "net_tx_bps", TendenciaMedia: "net_tx_avg",
		Valor: func(m database.MetricServer) (float64, bool) { return medido(m.NetTxBps) },
	},
	{
		Nome: "rtt", Rotulo: "RTT", Unidade: "ms", Escopo: EscopoServidor,
		SerieDoServidor: "rtt_ms", TendenciaMedia: "rtt_avg", TendenciaMaxima: "rtt_max",
		Valor: func(m database.MetricServer) (float64, bool) { return medido(m.RTTMs) },
	},
	{
		Nome: "latency", Rotulo: "Handshake SSH", Unidade: "ms", Escopo: EscopoServidor,
		SerieDoServidor: "ping_latency_ms",
	},
}

func Todas() []Metrica {
	return append([]Metrica(nil), registro...)
}

func Buscar(nome string) (Metrica, bool) {
	for _, m := range registro {
		if m.Nome == nome {
			return m, true
		}
	}
	return Metrica{}, false
}

func (m Metrica) TemTendencia() bool { return m.TendenciaMedia != "" }

func (m Metrica) Avaliavel() bool { return m.Valor != nil }

func Avaliavel(nome string) bool {
	m, ok := Buscar(nome)
	return ok && m.Avaliavel()
}
