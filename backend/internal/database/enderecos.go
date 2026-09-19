package database

import (
	"log"
	"net"
	"strings"
	"time"

	"gorm.io/gorm/clause"
)

const (
	OrigemColetado = "coletado"
	OrigemManual   = "manual"
)

type ServerAddress struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	ServerID      string    `gorm:"type:uuid;index:idx_server_addresses_srv_addr,unique,priority:1;not null" json:"server_id"`
	Address       string    `gorm:"size:45;index;index:idx_server_addresses_srv_addr,unique,priority:2;not null" json:"address"`
	Origem        string    `gorm:"size:16;not null;default:'coletado'" json:"origem"`
	PrimeiroVisto time.Time `json:"primeiro_visto"`
	UltimoVisto   time.Time `json:"ultimo_visto"`
}

func EnderecoValido(bruto string) (string, bool) {
	ip := net.ParseIP(bruto)
	if ip == nil {
		return "", false
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return "", false
	}
	if ip.To4() == nil {
		return "", false
	}
	return ip.String(), true
}

func RegistrarEnderecos(serverID string, brutos []string) {
	gravarEnderecos(serverID, brutos, OrigemColetado)
}

func RegistrarAliases(serverID string, brutos []string) {
	gravarEnderecos(serverID, brutos, OrigemManual)
}

func gravarEnderecos(serverID string, brutos []string, origem string) {
	if DB == nil || serverID == "" {
		return
	}

	agora := time.Now().UTC()
	vistos := make(map[string]bool, len(brutos))
	linhas := make([]ServerAddress, 0, len(brutos))
	for _, bruto := range brutos {
		endereco, ok := EnderecoValido(bruto)
		if !ok || vistos[endereco] {
			continue
		}
		vistos[endereco] = true
		linhas = append(linhas, ServerAddress{
			ServerID:      serverID,
			Address:       endereco,
			Origem:        origem,
			PrimeiroVisto: agora,
			UltimoVisto:   agora,
		})
	}
	if len(linhas) == 0 {
		return
	}

	atualiza := []string{"ultimo_visto"}
	if origem == OrigemManual {
		atualiza = append(atualiza, "origem")
	}
	err := DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "server_id"}, {Name: "address"}},
		DoUpdates: clause.AssignmentColumns(atualiza),
	}).Create(&linhas).Error
	if err != nil {
		log.Printf("[Enderecos] erro ao gravar endereços do servidor %s: %v", serverID, err)
	}
}

func EnderecosPorServidor(serverIDs []string) (map[string][]string, error) {
	fora := map[string][]string{}
	if DB == nil || len(serverIDs) == 0 {
		return fora, nil
	}

	var linhas []ServerAddress
	err := DB.Where("server_id IN ?", serverIDs).
		Order("origem DESC, address ASC").
		Find(&linhas).Error
	if err != nil {
		return nil, err
	}
	for _, l := range linhas {
		fora[l.ServerID] = append(fora[l.ServerID], l.Address)
	}
	return fora, nil
}

func PodarEnderecos(maxAge time.Duration) {
	if DB == nil {
		return
	}

	cutoff := time.Now().UTC().Add(-maxAge)
	res := DB.Where("origem = ? AND ultimo_visto < ?", OrigemColetado, cutoff).Delete(&ServerAddress{})
	if res.Error != nil {
		log.Printf("[Retention] erro ao podar server_addresses: %v", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		log.Printf("[Retention] server_addresses: %d endereços sem sinal removidos", res.RowsAffected)
	}
}

var overlayCGNAT = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

type DonoDeEndereco struct {
	ServerID string
	Nome     string
	SiteID   *uint
	Unidade  string
}

func EnderecoEhGlobal(endereco string) bool {
	ip := net.ParseIP(endereco)
	if ip == nil {
		return true
	}
	if overlayCGNAT.Contains(ip) {
		return true
	}
	return !ip.IsPrivate()
}

func DonoDoEndereco(endereco string, siteID *uint, exceto string) (DonoDeEndereco, bool) {
	if DB == nil || strings.TrimSpace(endereco) == "" {
		return DonoDeEndereco{}, false
	}

	q := DB.Table("servers").
		Select("servers.id AS server_id, servers.name AS nome, servers.site_id AS site_id, COALESCE(sites.name, '') AS unidade").
		Joins("LEFT JOIN sites ON sites.id = servers.site_id").
		Where(`servers.host_ip = ? OR EXISTS (
			SELECT 1 FROM server_addresses sa WHERE sa.server_id = servers.id AND sa.address = ?)`,
			endereco, endereco)

	if exceto != "" {
		q = q.Where("servers.id <> ?", exceto)
	}
	if !EnderecoEhGlobal(endereco) {
		if siteID == nil {
			q = q.Where("servers.site_id IS NULL")
		} else {
			q = q.Where("servers.site_id = ?", *siteID)
		}
	}

	var dono DonoDeEndereco
	if err := q.Limit(1).Scan(&dono).Error; err != nil {
		log.Printf("[Enderecos] erro ao procurar dono de %s: %v", endereco, err)
		return DonoDeEndereco{}, false
	}
	return dono, dono.ServerID != ""
}

func (d DonoDeEndereco) Descricao() string {
	if d.Unidade == "" {
		return d.Nome + " (sem unidade)"
	}
	return d.Nome + " (unidade " + d.Unidade + ")"
}

func UpstreamsRecentes(janela time.Duration) (map[string]bool, error) {
	fora := map[string]bool{}
	if DB == nil {
		return fora, nil
	}

	corte := time.Now().UTC().Add(-janela)
	var enderecos []string
	err := DB.Model(&MetricLoadBalancer{}).
		Distinct("split_part(upstream_addr, ':', 1)").
		Where("timestamp >= ?", corte).
		Pluck("split_part(upstream_addr, ':', 1)", &enderecos).Error
	if err != nil {
		return nil, err
	}
	for _, endereco := range enderecos {
		if endereco != "" {
			fora[endereco] = true
		}
	}
	return fora, nil
}
