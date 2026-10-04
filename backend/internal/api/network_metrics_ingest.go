package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jvS0uzx/dock_keeper/internal/audit"
	"github.com/jvS0uzx/dock_keeper/internal/database"
	"gorm.io/gorm"
)

const (
	esquemaDaTelemetriaDeRede = 1

	maxDispositivosPorEnvio  = 256
	maxInterfacesPorAparelho = 1024

	tetoSysDescr  = 255
	tetoSysName   = 255
	tetoInterface = 128
	tetoErroSNMP  = 200

	toleranciaDeRelogio = 5 * time.Minute

	loteDeMetricasDeInterface = 4000

	erroSNMPPadrao = "sem resposta SNMP"
)

var estadosDeInterface = map[string]string{
	"up":             "up",
	"down":           "down",
	"testing":        "testing",
	"unknown":        "unknown",
	"dormant":        "dormant",
	"notpresent":     "notPresent",
	"lowerlayerdown": "lowerLayerDown",
}

type interfaceNoEnvio struct {
	IfIndex     int      `json:"if_index"`
	IfName      string   `json:"if_name"`
	IfDescr     string   `json:"if_descr"`
	IfAlias     string   `json:"if_alias"`
	SpeedMbps   *int64   `json:"speed_mbps"`
	OperStatus  string   `json:"oper_status"`
	AdminStatus string   `json:"admin_status"`
	InBps       *float64 `json:"in_bps"`
	OutBps      *float64 `json:"out_bps"`
	InErrors    *int64   `json:"in_errors"`
	OutErrors   *int64   `json:"out_errors"`
	InDiscards  *int64   `json:"in_discards"`
	OutDiscards *int64   `json:"out_discards"`
}

type aparelhoNoEnvio struct {
	IP         string             `json:"ip"`
	Reachable  bool               `json:"reachable"`
	Error      string             `json:"error"`
	SysName    string             `json:"sys_name"`
	SysDescr   string             `json:"sys_descr"`
	UptimeSec  *int64             `json:"uptime_sec"`
	Interfaces []interfaceNoEnvio `json:"interfaces"`
}

type envioDeTelemetriaDeRede struct {
	Schema           int               `json:"schema"`
	SiteCode         string            `json:"site_code"`
	CollectorVersion string            `json:"collector_version"`
	IntervalSec      int               `json:"interval_sec"`
	CollectedAt      string            `json:"collected_at"`
	Devices          []aparelhoNoEnvio `json:"devices"`
}

var errEnvioGrandeDemais = errors.New("telemetria de rede grande demais")

func NetworkMetricsIngestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if recusasAnonimasNoTeto(w, r) {
		return
	}
	cred, err := authenticateDevice(r)
	if err != nil {
		contarRecusaAnonima(r)
		refuseDeviceAuth(w, r, err, "network_metrics.legacy_token_disabled")
		return
	}
	if !cred.allowsKind(kindCollector) {
		refuseDeviceKind(w, r, cred, "network_metrics.kind_mismatch", kindCollector, "telemetria de rede")
		return
	}
	if !limitarTaxa(w, r, chaveDeIngestao(r, cred), tetoDeIngestao()) {
		return
	}

	var p envioDeTelemetriaDeRede
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if p.Schema != esquemaDaTelemetriaDeRede {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("schema %d não suportado: este painel aceita o schema %d", p.Schema, esquemaDaTelemetriaDeRede))
		return
	}
	if err := validarEnvioDeRede(&p); err != nil {
		if errors.Is(err, errEnvioGrandeDemais) {
			writeError(w, http.StatusRequestEntityTooLarge, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	siteID, err := resolveInventorySite(cred, p.SiteCode)
	if err != nil {
		if errors.Is(err, errSiteMismatch) {
			auditarUnidadeDivergenteNaTelemetria(cred, p)
			writeError(w, http.StatusConflict, "unidade declarada não confere com a credencial do dispositivo")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	agora := time.Now().UTC()
	coletadoEm := horarioDaColeta(p.CollectedAt, agora)

	interfaces, err := gravarTelemetriaDeRede(r, p.Devices, *siteID, coletadoEm, agora)
	if err != nil {
		log.Printf("[Rede] erro ao gravar a telemetria SNMP da unidade %q: %v", p.SiteCode, err)
		writeError(w, http.StatusInternalServerError, "falha ao gravar a telemetria de rede")
		return
	}

	writeJSON(w, http.StatusOK, map[string]int{"devices": len(p.Devices), "interfaces": interfaces})
}

func validarEnvioDeRede(p *envioDeTelemetriaDeRede) error {
	if len(p.Devices) > maxDispositivosPorEnvio {
		return fmt.Errorf("%w: %d dispositivos, o teto é %d", errEnvioGrandeDemais, len(p.Devices), maxDispositivosPorEnvio)
	}
	for i := range p.Devices {
		d := &p.Devices[i]
		d.IP = strings.TrimSpace(d.IP)
		ip := net.ParseIP(d.IP)
		if ip == nil {
			return fmt.Errorf("ip inválido no dispositivo %d: %q", i, d.IP)
		}
		d.IP = ip.String()
		if len(d.Interfaces) > maxInterfacesPorAparelho {
			return fmt.Errorf("%w: %d interfaces em %s, o teto é %d", errEnvioGrandeDemais, len(d.Interfaces), d.IP, maxInterfacesPorAparelho)
		}
		d.SysName = textoSeguro(d.SysName, tetoSysName)
		d.SysDescr = textoSeguro(d.SysDescr, tetoSysDescr)
		d.Error = textoSeguro(d.Error, tetoErroSNMP)
		d.UptimeSec = semNegativo(d.UptimeSec)

		for j := range d.Interfaces {
			itf := &d.Interfaces[j]
			if negativo(itf.InBps) || negativo(itf.OutBps) {
				return fmt.Errorf("bps negativo na interface %d de %s", itf.IfIndex, d.IP)
			}
			itf.IfName = textoSeguro(itf.IfName, tetoInterface)
			itf.IfDescr = textoSeguro(itf.IfDescr, tetoInterface)
			itf.IfAlias = textoSeguro(itf.IfAlias, tetoInterface)
			itf.OperStatus = estadoDeInterface(itf.OperStatus)
			itf.AdminStatus = estadoDeInterface(itf.AdminStatus)
			itf.SpeedMbps = semNegativo(itf.SpeedMbps)
			itf.InErrors = semNegativo(itf.InErrors)
			itf.OutErrors = semNegativo(itf.OutErrors)
			itf.InDiscards = semNegativo(itf.InDiscards)
			itf.OutDiscards = semNegativo(itf.OutDiscards)
		}
	}
	return nil
}

func negativo(v *float64) bool {
	return v != nil && *v < 0
}

func semNegativo(v *int64) *int64 {
	if v == nil || *v < 0 {
		return nil
	}
	return v
}

func textoSeguro(s string, teto int) string {
	s = strings.TrimSpace(strings.ToValidUTF8(strings.ReplaceAll(s, "\x00", ""), ""))
	if utf8.RuneCountInString(s) <= teto {
		return s
	}
	return string([]rune(s)[:teto])
}

func estadoDeInterface(s string) string {
	if canonico, ok := estadosDeInterface[strings.ToLower(strings.TrimSpace(s))]; ok {
		return canonico
	}
	return "unknown"
}

func horarioDaColeta(declarado string, agora time.Time) time.Time {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(declarado))
	if err != nil || t.After(agora.Add(toleranciaDeRelogio)) {
		return agora
	}
	return t.UTC()
}

func gravarTelemetriaDeRede(r *http.Request, aparelhos []aparelhoNoEnvio, siteID uint, coletadoEm, agora time.Time) (int, error) {
	if len(aparelhos) == 0 {
		return 0, nil
	}
	ips := make([]string, 0, len(aparelhos))
	for _, a := range aparelhos {
		ips = append(ips, a.IP)
	}
	if err := database.AdoptNetworkHostsWithoutSite(siteID, ips); err != nil {
		return 0, err
	}

	total := 0
	err := database.From(r.Context()).Transaction(func(tx *gorm.DB) error {
		var metricas []database.MetricNetworkInterface
		for _, a := range aparelhos {
			hostID, err := registrarAparelhoSNMP(tx, a, siteID, coletadoEm, agora)
			if err != nil {
				return err
			}
			if !a.Reachable {
				continue
			}
			novas, err := reconciliarInterfaces(tx, hostID, a.Interfaces, coletadoEm, agora)
			if err != nil {
				return err
			}
			total += len(novas)
			metricas = append(metricas, novas...)
		}
		if len(metricas) == 0 {
			return nil
		}
		return tx.CreateInBatches(&metricas, loteDeMetricasDeInterface).Error
	})
	return total, err
}

func registrarAparelhoSNMP(tx *gorm.DB, a aparelhoNoEnvio, siteID uint, coletadoEm, agora time.Time) (uint, error) {
	var id uint
	if a.Reachable {
		err := tx.Raw(`
			INSERT INTO network_hosts (ip, site_id, hostname, mac, open_ports, device_type, first_seen, last_seen,
				snmp_sys_name, snmp_sys_descr, snmp_uptime_sec, snmp_visto_em, snmp_erro, snmp_erro_em)
			VALUES (?, ?, ?, '', '', '', ?, ?, ?, ?, ?, ?, '', NULL)
			ON CONFLICT (COALESCE(site_id, 0), ip) DO UPDATE SET
				last_seen = EXCLUDED.last_seen,
				snmp_sys_name = EXCLUDED.snmp_sys_name,
				snmp_sys_descr = EXCLUDED.snmp_sys_descr,
				snmp_uptime_sec = EXCLUDED.snmp_uptime_sec,
				snmp_visto_em = EXCLUDED.snmp_visto_em,
				snmp_erro = '',
				snmp_erro_em = NULL
			RETURNING id`,
			a.IP, siteID, a.SysName, agora, agora, a.SysName, a.SysDescr, a.UptimeSec, coletadoEm).Scan(&id).Error
		return id, err
	}

	motivo := a.Error
	if motivo == "" {
		motivo = erroSNMPPadrao
	}
	err := tx.Raw(`
		INSERT INTO network_hosts (ip, site_id, hostname, mac, open_ports, device_type, first_seen, last_seen,
			snmp_erro, snmp_erro_em)
		VALUES (?, ?, '', '', '', '', ?, ?, ?, ?)
		ON CONFLICT (COALESCE(site_id, 0), ip) DO UPDATE SET
			snmp_erro = EXCLUDED.snmp_erro,
			snmp_erro_em = EXCLUDED.snmp_erro_em
		RETURNING id`,
		a.IP, siteID, agora, agora, motivo, coletadoEm).Scan(&id).Error
	return id, err
}

type interfaceConhecida struct {
	ID      uint
	IfIndex int
	IfName  string
}

func reconciliarInterfaces(tx *gorm.DB, hostID uint, recebidas []interfaceNoEnvio, coletadoEm, agora time.Time) ([]database.MetricNetworkInterface, error) {
	var conhecidas []interfaceConhecida
	if err := tx.Table("network_interfaces").Select("id, if_index, if_name").
		Where("network_host_id = ?", hostID).Order("id").Scan(&conhecidas).Error; err != nil {
		return nil, err
	}

	porNome := map[string]uint{}
	semNomePorIndice := map[int]uint{}
	porIndice := map[int]uint{}
	for _, c := range conhecidas {
		if c.IfName != "" {
			porNome[c.IfName] = c.ID
		} else if _, ok := semNomePorIndice[c.IfIndex]; !ok {
			semNomePorIndice[c.IfIndex] = c.ID
		}
		if _, ok := porIndice[c.IfIndex]; !ok {
			porIndice[c.IfIndex] = c.ID
		}
	}

	usadas := map[uint]bool{}
	nomesNoEnvio := map[string]bool{}
	indicesSemNome := map[int]bool{}
	casar := func(itf interfaceNoEnvio) uint {
		candidatos := []map[int]uint{semNomePorIndice}
		if itf.IfName != "" {
			if id, ok := porNome[itf.IfName]; ok && !usadas[id] {
				return id
			}
		} else {
			candidatos = append(candidatos, porIndice)
		}
		for _, mapa := range candidatos {
			if id, ok := mapa[itf.IfIndex]; ok && !usadas[id] {
				return id
			}
		}
		return 0
	}

	var atualizar []interfaceNoEnvio
	var idsAtualizar []uint
	var criar []database.NetworkInterface
	var leiturasNovas []interfaceNoEnvio

	for _, itf := range recebidas {
		if itf.IfName == "" && itf.IfIndex <= 0 {
			continue
		}
		if itf.IfName != "" {
			if nomesNoEnvio[itf.IfName] {
				continue
			}
			nomesNoEnvio[itf.IfName] = true
		} else {
			if indicesSemNome[itf.IfIndex] {
				continue
			}
			indicesSemNome[itf.IfIndex] = true
		}

		if id := casar(itf); id != 0 {
			usadas[id] = true
			atualizar = append(atualizar, itf)
			idsAtualizar = append(idsAtualizar, id)
			continue
		}
		criar = append(criar, database.NetworkInterface{
			NetworkHostID: hostID,
			IfIndex:       itf.IfIndex,
			IfName:        itf.IfName,
			IfDescr:       itf.IfDescr,
			IfAlias:       itf.IfAlias,
			SpeedMbps:     itf.SpeedMbps,
			OperStatus:    itf.OperStatus,
			AdminStatus:   itf.AdminStatus,
			FirstSeen:     agora,
			LastSeen:      agora,
		})
		leiturasNovas = append(leiturasNovas, itf)
	}

	if err := atualizarInterfaces(tx, atualizar, idsAtualizar, agora); err != nil {
		return nil, err
	}
	if len(criar) > 0 {
		if err := tx.Create(&criar).Error; err != nil {
			return nil, err
		}
	}

	metricas := make([]database.MetricNetworkInterface, 0, len(atualizar)+len(leiturasNovas))
	for i, itf := range atualizar {
		metricas = append(metricas, leituraDeInterface(idsAtualizar[i], itf, coletadoEm))
	}
	for i, itf := range leiturasNovas {
		metricas = append(metricas, leituraDeInterface(criar[i].ID, itf, coletadoEm))
	}
	return metricas, nil
}

func atualizarInterfaces(tx *gorm.DB, lista []interfaceNoEnvio, ids []uint, agora time.Time) error {
	if len(lista) == 0 {
		return nil
	}
	valores := make([]string, 0, len(lista))
	args := make([]any, 0, len(lista)*8+1)
	args = append(args, agora)
	for i, itf := range lista {
		valores = append(valores, "(?::bigint, ?::integer, ?::text, ?::text, ?::text, ?::bigint, ?::text, ?::text)")
		args = append(args, ids[i], itf.IfIndex, itf.IfName, itf.IfDescr, itf.IfAlias, itf.SpeedMbps, itf.OperStatus, itf.AdminStatus)
	}
	sql := `
		UPDATE network_interfaces AS n SET
			if_index = v.if_index,
			if_name = CASE WHEN v.if_name <> '' THEN v.if_name ELSE n.if_name END,
			if_descr = v.if_descr,
			if_alias = v.if_alias,
			speed_mbps = v.speed_mbps,
			oper_status = v.oper_status,
			admin_status = v.admin_status,
			last_seen = ?
		FROM (VALUES ` + strings.Join(valores, ", ") + `) AS v(id, if_index, if_name, if_descr, if_alias, speed_mbps, oper_status, admin_status)
		WHERE n.id = v.id`
	return tx.Exec(sql, args...).Error
}

func leituraDeInterface(id uint, itf interfaceNoEnvio, coletadoEm time.Time) database.MetricNetworkInterface {
	return database.MetricNetworkInterface{
		InterfaceID: id,
		Ts:          coletadoEm,
		InBps:       itf.InBps,
		OutBps:      itf.OutBps,
		InErrors:    itf.InErrors,
		OutErrors:   itf.OutErrors,
		InDiscards:  itf.InDiscards,
		OutDiscards: itf.OutDiscards,
		OperStatus:  itf.OperStatus,
	}
}

func auditarUnidadeDivergenteNaTelemetria(cred deviceAuth, p envioDeTelemetriaDeRede) {
	audit.Record(audit.Entry{
		Action:     "network_metrics.site_mismatch",
		TargetType: "device",
		TargetID:   cred.DeviceID,
		SiteID:     cred.SiteID,
		Result:     audit.ResultDenied,
		Detail: map[string]any{
			"site_code_no_envio":      p.SiteCode,
			"dispositivos_no_envio":   len(p.Devices),
			"collector_version":       p.CollectorVersion,
			"schema_da_telemetria":    p.Schema,
			"intervalo_declarado_seg": p.IntervalSec,
		},
	})
}
