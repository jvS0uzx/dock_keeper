package database

import (
	"errors"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	OrigemColetado = "coletado"
	OrigemManual   = "manual"
)

const MaxEnderecosPorEnvio = 32

const JanelaDeDonoColetado = 15 * time.Minute

const (
	janelaDoAvisoDeRecusa = time.Hour
	tetoDeAvisosDeRecusa  = 4096
)

type EnderecoRecusado struct {
	Endereco string
	Dono     DonoDeEndereco
}

var AoRecusarEndereco func(serverID string, recusados []EnderecoRecusado)

var avisosDeRecusa = struct {
	mu     sync.Mutex
	vistos map[string]time.Time
}{vistos: map[string]time.Time{}}

func avisoInedito(chave string, agora time.Time) bool {
	avisosDeRecusa.mu.Lock()
	defer avisosDeRecusa.mu.Unlock()

	if visto, ok := avisosDeRecusa.vistos[chave]; ok && agora.Sub(visto) < janelaDoAvisoDeRecusa {
		return false
	}
	if len(avisosDeRecusa.vistos) >= tetoDeAvisosDeRecusa {
		for k, visto := range avisosDeRecusa.vistos {
			if agora.Sub(visto) >= janelaDoAvisoDeRecusa {
				delete(avisosDeRecusa.vistos, k)
			}
		}
		if len(avisosDeRecusa.vistos) >= tetoDeAvisosDeRecusa {
			avisosDeRecusa.vistos = map[string]time.Time{}
		}
	}
	avisosDeRecusa.vistos[chave] = agora
	return true
}

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
	gravarEnderecos(serverID, brutos, OrigemColetado, "")
}

func RegistrarEnderecosDoAgente(serverID string, declarados []string, observado string) {
	silencioso, ok := EnderecoValido(observado)
	if !ok {
		gravarEnderecos(serverID, declarados, OrigemColetado, "")
		return
	}
	for _, bruto := range declarados {
		if endereco, valido := EnderecoValido(bruto); valido && endereco == silencioso {
			silencioso = ""
			break
		}
	}
	gravarEnderecos(serverID, append(append([]string{}, declarados...), observado), OrigemColetado, silencioso)
}

func RegistrarAliases(serverID string, brutos []string) {
	gravarEnderecos(serverID, brutos, OrigemManual, "")
}

func SubstituirAliases(serverID string, aliases []string) error {
	if DB == nil || serverID == "" {
		return ErrBancoIndisponivel
	}

	remover := DB.Where("server_id = ? AND origem = ?", serverID, OrigemManual)
	if len(aliases) > 0 {
		remover = remover.Where("address NOT IN ?", aliases)
	}
	if err := remover.Delete(&ServerAddress{}).Error; err != nil {
		return err
	}
	RegistrarAliases(serverID, aliases)
	return nil
}

func AliasesPorServidor(serverIDs []string) (map[string][]string, error) {
	fora := map[string][]string{}
	if DB == nil || len(serverIDs) == 0 {
		return fora, nil
	}

	var linhas []ServerAddress
	err := DB.Where("server_id IN ? AND origem = ?", serverIDs, OrigemManual).
		Order("address ASC").Find(&linhas).Error
	if err != nil {
		return nil, err
	}
	for _, l := range linhas {
		fora[l.ServerID] = append(fora[l.ServerID], l.Address)
	}
	return fora, nil
}

func gravarEnderecos(serverID string, brutos []string, origem, silencioso string) {
	if DB == nil || serverID == "" {
		return
	}

	agora := time.Now().UTC()
	vistos := make(map[string]bool, len(brutos))
	enderecos := make([]string, 0, len(brutos))
	for _, bruto := range brutos {
		endereco, ok := EnderecoValido(bruto)
		if !ok || vistos[endereco] {
			continue
		}
		vistos[endereco] = true
		enderecos = append(enderecos, endereco)
	}

	if origem == OrigemColetado {
		if excedente := len(enderecos) - MaxEnderecosPorEnvio; excedente > 0 {
			enderecos = enderecos[:MaxEnderecosPorEnvio]
			if avisoInedito(serverID+"|#teto", agora) {
				log.Printf("[Enderecos] servidor %s declarou endereços demais: %d descartados acima do teto de %d",
					serverID, excedente, MaxEnderecosPorEnvio)
			}
		}

		aceitos, recusados, err := filtrarPorDono(serverID, enderecos, agora)
		if err != nil {
			log.Printf("[Enderecos] erro ao conferir o dono dos endereços de %s; nada foi gravado: %v", serverID, err)
			return
		}
		enderecos = aceitos
		avisarRecusa(serverID, semOSilencioso(recusados, silencioso), agora)
	}
	if len(enderecos) == 0 {
		return
	}

	linhas := make([]ServerAddress, 0, len(enderecos))
	for _, endereco := range enderecos {
		linhas = append(linhas, ServerAddress{
			ServerID:      serverID,
			Address:       endereco,
			Origem:        origem,
			PrimeiroVisto: agora,
			UltimoVisto:   agora,
		})
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

type candidatoADono struct {
	Endereco string
	ServerID string
	Nome     string
	SiteID   *uint
	Unidade  string
	Forte    bool
	Visto    time.Time
	Coletado bool
}

const consultaDeCandidatosADono = `
	SELECT c.address AS endereco, s.id AS server_id, s.name AS nome, s.site_id AS site_id,
	       COALESCE(sites.name, '') AS unidade, c.forte AS forte, c.visto AS visto, c.coletado AS coletado
	FROM (
		SELECT host_ip AS address, id AS server_id, (COALESCE(kind, 'ssh') <> 'agent') AS forte,
		       updated_at AS visto, false AS coletado
		FROM servers
		WHERE deleted_at IS NULL AND host_ip IN ?
		UNION ALL
		SELECT sa.address, sa.server_id, (sa.origem = 'manual'), sa.ultimo_visto, (sa.origem = 'coletado')
		FROM server_addresses sa
		WHERE sa.address IN ?
	) c
	JOIN servers s ON s.id = c.server_id AND s.deleted_at IS NULL
	LEFT JOIN sites ON sites.id = s.site_id
	WHERE c.server_id <> ?
	ORDER BY c.forte DESC, c.visto DESC`

func filtrarPorDono(serverID string, enderecos []string, agora time.Time) ([]string, []EnderecoRecusado, error) {
	if len(enderecos) == 0 {
		return nil, nil, nil
	}

	var eu Server
	if err := DB.Select("id", "site_id").Where("id = ?", serverID).Take(&eu).Error; err != nil {
		return nil, nil, err
	}

	var candidatos []candidatoADono
	if err := DB.Raw(consultaDeCandidatosADono, enderecos, enderecos, serverID).Scan(&candidatos).Error; err != nil {
		return nil, nil, err
	}

	corte := agora.Add(-JanelaDeDonoColetado)
	donos := map[string]DonoDeEndereco{}
	parados := map[string][]string{}
	for _, c := range candidatos {
		if !EnderecoEhGlobal(c.Endereco) && !mesmaUnidade(c.SiteID, eu.SiteID) {
			continue
		}
		if c.Forte || !c.Visto.Before(corte) {
			if _, ja := donos[c.Endereco]; !ja {
				donos[c.Endereco] = DonoDeEndereco{ServerID: c.ServerID, Nome: c.Nome, SiteID: c.SiteID, Unidade: c.Unidade}
			}
			continue
		}
		if c.Coletado {
			parados[c.Endereco] = append(parados[c.Endereco], c.ServerID)
		}
	}

	var aceitos []string
	var recusados []EnderecoRecusado
	for _, endereco := range enderecos {
		if dono, tem := donos[endereco]; tem {
			recusados = append(recusados, EnderecoRecusado{Endereco: endereco, Dono: dono})
			continue
		}
		if antigos := parados[endereco]; len(antigos) > 0 {
			err := DB.Where("address = ? AND origem = ? AND server_id IN ?", endereco, OrigemColetado, antigos).
				Delete(&ServerAddress{}).Error
			if err != nil {
				return nil, nil, err
			}
		}
		aceitos = append(aceitos, endereco)
	}
	return aceitos, recusados, nil
}

func mesmaUnidade(a, b *uint) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func semOSilencioso(recusados []EnderecoRecusado, silencioso string) []EnderecoRecusado {
	if silencioso == "" {
		return recusados
	}
	var fora []EnderecoRecusado
	for _, r := range recusados {
		if r.Endereco != silencioso {
			fora = append(fora, r)
		}
	}
	return fora
}

func avisarRecusa(serverID string, recusados []EnderecoRecusado, agora time.Time) {
	var ineditos []EnderecoRecusado
	for _, r := range recusados {
		if avisoInedito(serverID+"|"+r.Endereco, agora) {
			ineditos = append(ineditos, r)
		}
	}
	if len(ineditos) == 0 {
		return
	}
	for _, r := range ineditos {
		log.Printf("[Enderecos] servidor %s declarou %s, que pertence a %s; endereço recusado",
			serverID, r.Endereco, r.Dono.Descricao())
	}
	if AoRecusarEndereco != nil {
		AoRecusarEndereco(serverID, ineditos)
	}
}

func LimparEnderecosDoServidor(tx *gorm.DB, serverID string) error {
	return tx.Where("server_id = ?", serverID).Delete(&ServerAddress{}).Error
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

var ErrBancoIndisponivel = errors.New("banco indisponível")

func DonoDoEndereco(endereco string, siteID *uint, exceto string) (DonoDeEndereco, bool, error) {
	if strings.TrimSpace(endereco) == "" {
		return DonoDeEndereco{}, false, nil
	}
	if DB == nil {
		return DonoDeEndereco{}, false, ErrBancoIndisponivel
	}

	q := DB.Table("servers").
		Select("servers.id AS server_id, servers.name AS nome, servers.site_id AS site_id, COALESCE(sites.name, '') AS unidade").
		Joins("LEFT JOIN sites ON sites.id = servers.site_id").
		Where("servers.deleted_at IS NULL").
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
		return DonoDeEndereco{}, false, err
	}
	return dono, dono.ServerID != "", nil
}

func (d DonoDeEndereco) Descricao() string {
	if d.Unidade == "" {
		return d.Nome + " (sem unidade)"
	}
	return d.Nome + " (unidade " + d.Unidade + ")"
}

func UpstreamsDeclarados() (map[string]bool, error) {
	fora := map[string]bool{}
	if DB == nil {
		return fora, nil
	}

	var enderecos []string
	err := DB.Model(&NginxUpstream{}).
		Distinct("split_part(destino, ':', 1)").
		Pluck("split_part(destino, ':', 1)", &enderecos).Error
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
