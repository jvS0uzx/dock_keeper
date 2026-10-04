package api

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const (
	tetoDeInterfacesNaLeitura = 4096
	tetoDePontosDaSerie       = 2000
)

type janelaDaSerie struct {
	duracao time.Duration
	balde   int
}

var janelasDaSerie = map[string]janelaDaSerie{
	"1h":  {duracao: time.Hour, balde: 60},
	"6h":  {duracao: 6 * time.Hour, balde: 60},
	"24h": {duracao: 24 * time.Hour, balde: 300},
	"72h": {duracao: 72 * time.Hour, balde: 900},
}

type HostSNMPView struct {
	ID            uint       `json:"id"`
	IP            string     `json:"ip"`
	Hostname      string     `json:"hostname"`
	SnmpSysName   string     `json:"snmp_sys_name"`
	SnmpSysDescr  string     `json:"snmp_sys_descr"`
	SnmpUptimeSec *int64     `json:"snmp_uptime_sec"`
	SnmpVistoEm   *time.Time `json:"snmp_visto_em"`
	SnmpErro      string     `json:"snmp_erro"`
	SnmpErroEm    *time.Time `json:"snmp_erro_em"`
}

type LeituraDeInterfaceView struct {
	Ts          time.Time `json:"ts"`
	InBps       *float64  `json:"in_bps"`
	OutBps      *float64  `json:"out_bps"`
	InErrors    *int64    `json:"in_errors"`
	OutErrors   *int64    `json:"out_errors"`
	InDiscards  *int64    `json:"in_discards"`
	OutDiscards *int64    `json:"out_discards"`
}

type InterfaceView struct {
	ID          uint                    `json:"id"`
	IfIndex     int                     `json:"if_index"`
	IfName      string                  `json:"if_name"`
	IfDescr     string                  `json:"if_descr"`
	IfAlias     string                  `json:"if_alias"`
	SpeedMbps   *int64                  `json:"speed_mbps"`
	OperStatus  string                  `json:"oper_status"`
	AdminStatus string                  `json:"admin_status"`
	LastSeen    time.Time               `json:"last_seen"`
	Ultima      *LeituraDeInterfaceView `json:"ultima"`
}

type PontoDaSerieDeInterface struct {
	Ts     time.Time `json:"ts"`
	InBps  *float64  `json:"in_bps"`
	OutBps *float64  `json:"out_bps"`
}

type linhaDeInterface struct {
	database.NetworkInterface
	UltimaTs          *time.Time
	UltimaInBps       *float64
	UltimaOutBps      *float64
	UltimaInErrors    *int64
	UltimaOutErrors   *int64
	UltimaInDiscards  *int64
	UltimaOutDiscards *int64
}

func idDoCaminho(r *http.Request) (uint, bool) {
	n, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || n == 0 {
		return 0, false
	}
	return uint(n), true
}

func hostNoAlcance(r *http.Request, id uint) (database.NetworkHost, int) {
	scope, status := resolveScope(sessionFrom(r), r)
	if status != 0 {
		return database.NetworkHost{}, status
	}
	var host database.NetworkHost
	if err := scope.apply(database.From(r.Context()).Model(&database.NetworkHost{})).
		Where("id = ?", id).First(&host).Error; err != nil {
		return database.NetworkHost{}, http.StatusNotFound
	}
	return host, 0
}

func interfacesDoHostHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := idDoCaminho(r)
	if !ok {
		writeError(w, http.StatusNotFound, "host não encontrado no inventário")
		return
	}
	host, status := hostNoAlcance(r, id)
	if status == http.StatusNotFound {
		writeError(w, status, "host não encontrado no inventário")
		return
	}
	if status != 0 {
		writeError(w, status, "site_id inválido ou fora do seu alcance")
		return
	}

	var linhas []linhaDeInterface
	err := database.From(r.Context()).Raw(`
		SELECT i.*, m.ts AS ultima_ts, m.in_bps AS ultima_in_bps, m.out_bps AS ultima_out_bps,
			m.in_errors AS ultima_in_errors, m.out_errors AS ultima_out_errors,
			m.in_discards AS ultima_in_discards, m.out_discards AS ultima_out_discards
		FROM network_interfaces AS i
		LEFT JOIN LATERAL (
			SELECT ts, in_bps, out_bps, in_errors, out_errors, in_discards, out_discards
			FROM metric_network_interfaces
			WHERE interface_id = i.id
			ORDER BY ts DESC
			LIMIT 1
		) AS m ON true
		WHERE i.network_host_id = ?
		ORDER BY i.if_index, i.id
		LIMIT ?`, host.ID, tetoDeInterfacesNaLeitura).Scan(&linhas).Error
	if err != nil {
		log.Printf("[Rede] erro ao ler as interfaces do host %d: %v", host.ID, err)
		writeError(w, http.StatusInternalServerError, "falha ao ler as interfaces")
		return
	}

	interfaces := make([]InterfaceView, 0, len(linhas))
	for _, l := range linhas {
		v := InterfaceView{
			ID:          l.ID,
			IfIndex:     l.IfIndex,
			IfName:      l.IfName,
			IfDescr:     l.IfDescr,
			IfAlias:     l.IfAlias,
			SpeedMbps:   l.SpeedMbps,
			OperStatus:  l.OperStatus,
			AdminStatus: l.AdminStatus,
			LastSeen:    l.LastSeen,
		}
		if l.UltimaTs != nil {
			v.Ultima = &LeituraDeInterfaceView{
				Ts:          *l.UltimaTs,
				InBps:       l.UltimaInBps,
				OutBps:      l.UltimaOutBps,
				InErrors:    l.UltimaInErrors,
				OutErrors:   l.UltimaOutErrors,
				InDiscards:  l.UltimaInDiscards,
				OutDiscards: l.UltimaOutDiscards,
			}
		}
		interfaces = append(interfaces, v)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"host": HostSNMPView{
			ID:            host.ID,
			IP:            host.IP,
			Hostname:      host.Hostname,
			SnmpSysName:   host.SnmpSysName,
			SnmpSysDescr:  host.SnmpSysDescr,
			SnmpUptimeSec: host.SnmpUptimeSec,
			SnmpVistoEm:   host.SnmpVistoEm,
			SnmpErro:      host.SnmpErro,
			SnmpErroEm:    host.SnmpErroEm,
		},
		"interfaces": interfaces,
	})
}

func serieDaInterfaceHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := idDoCaminho(r)
	if !ok {
		writeError(w, http.StatusNotFound, "interface não encontrada")
		return
	}

	chave := r.URL.Query().Get("janela")
	if chave == "" {
		chave = "1h"
	}
	janela, ok := janelasDaSerie[chave]
	if !ok {
		writeError(w, http.StatusBadRequest, "janela inválida: use 1h, 6h, 24h ou 72h")
		return
	}

	var itf database.NetworkInterface
	if err := database.From(r.Context()).Where("id = ?", id).First(&itf).Error; err != nil {
		writeError(w, http.StatusNotFound, "interface não encontrada")
		return
	}
	if _, status := hostNoAlcance(r, itf.NetworkHostID); status != 0 {
		if status == http.StatusNotFound {
			writeError(w, status, "interface não encontrada")
			return
		}
		writeError(w, status, "site_id inválido ou fora do seu alcance")
		return
	}

	balde := fmt.Sprintf("to_timestamp(floor(extract(epoch from ts) / %d) * %d)", janela.balde, janela.balde)
	inicio := time.Now().UTC().Add(-janela.duracao)

	var pontos []PontoDaSerieDeInterface
	err := database.From(r.Context()).Raw(`
		SELECT `+balde+` AS ts, AVG(in_bps) AS in_bps, AVG(out_bps) AS out_bps
		FROM metric_network_interfaces
		WHERE interface_id = ? AND ts >= ?
		GROUP BY 1
		ORDER BY 1
		LIMIT ?`, itf.ID, inicio, tetoDePontosDaSerie).Scan(&pontos).Error
	if err != nil {
		log.Printf("[Rede] erro ao ler a série da interface %d: %v", itf.ID, err)
		writeError(w, http.StatusInternalServerError, "falha ao ler a série da interface")
		return
	}
	if pontos == nil {
		pontos = []PontoDaSerieDeInterface{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"pontos": pontos})
}
