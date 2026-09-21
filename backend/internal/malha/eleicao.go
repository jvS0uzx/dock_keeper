package malha

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/alert"
	"github.com/jvS0uzx/dock_keeper/internal/config"
	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/safego"
)

const (
	janelaPadrao    = 5 * time.Minute
	intervaloPadrao = time.Minute
	margemPadraoPct = 25
	ciclosPadrao    = 2
	semUnidade      = "sem-unidade"
)

type Membro struct {
	ID          string
	Nome        string
	Candidato   bool
	Requisicoes int
	PapelAtual  string
}

type Decisao struct {
	Papeis    map[string]string
	Principal string
	Anterior  string
	Desafios  int
}

func (d Decisao) Trocou() bool {
	return d.Principal != d.Anterior
}

var (
	mu       sync.Mutex
	desafios = map[string]int{}
)

func JanelaDeTrafego() time.Duration {
	return database.EnvDuration("LB_WINDOW", janelaPadrao)
}

func IntervaloDaEleicao() time.Duration {
	return database.EnvDuration("MALHA_ELEICAO_INTERVAL", intervaloPadrao)
}

func margemDeTroca() float64 {
	pct := config.Inteiro("MALHA_MARGEM_TROCA_PCT", margemPadraoPct)
	if pct < 0 {
		pct = margemPadraoPct
	}
	return 1 + float64(pct)/100
}

func ciclosParaTrocar() int {
	ciclos := config.Inteiro("MALHA_CICLOS_TROCA", ciclosPadrao)
	if ciclos < 1 {
		return 1
	}
	return ciclos
}

func liderUnico(candidatos []*Membro) *Membro {
	var primeiro, segundo *Membro
	for _, m := range candidatos {
		if primeiro == nil || m.Requisicoes > primeiro.Requisicoes {
			segundo = primeiro
			primeiro = m
			continue
		}
		if segundo == nil || m.Requisicoes > segundo.Requisicoes {
			segundo = m
		}
	}
	if primeiro == nil || primeiro.Requisicoes == 0 {
		return nil
	}
	if segundo != nil && segundo.Requisicoes == primeiro.Requisicoes {
		return nil
	}
	return primeiro
}

func decidir(membros []Membro, desafiosAtuais, ciclos int, margem float64) Decisao {
	d := Decisao{Papeis: make(map[string]string, len(membros)), Desafios: desafiosAtuais}

	var incumbente *Membro
	candidatos := make([]*Membro, 0, len(membros))
	total := 0
	for i := range membros {
		m := &membros[i]
		d.Papeis[m.ID] = database.NginxPapelNenhum
		if m.PapelAtual == database.NginxPapelPrincipal {
			incumbente = m
		}
		if m.Candidato {
			candidatos = append(candidatos, m)
			total += m.Requisicoes
		}
	}
	for _, m := range candidatos {
		d.Papeis[m.ID] = database.NginxPapelReserva
	}
	if incumbente != nil {
		d.Anterior = incumbente.ID
	}

	incumbenteValido := incumbente != nil && incumbente.Candidato

	switch {
	case len(candidatos) == 0:
		d.Desafios = 0

	case total == 0:
		if incumbenteValido {
			d.Principal = incumbente.ID
			d.Desafios = 0
			break
		}
		if incumbente == nil {
			d.Desafios = 0
			break
		}
		d.Desafios++
		if d.Desafios < ciclos {
			d.Principal = incumbente.ID
			break
		}
		d.Desafios = 0

	default:
		lider := liderUnico(candidatos)
		switch {
		case lider == nil:
			d.Desafios = 0
			if incumbenteValido {
				d.Principal = incumbente.ID
			}

		case incumbenteValido && lider.ID == incumbente.ID:
			d.Desafios = 0
			d.Principal = incumbente.ID

		case incumbenteValido && float64(lider.Requisicoes) <= float64(incumbente.Requisicoes)*margem:
			d.Desafios = 0
			d.Principal = incumbente.ID

		default:
			d.Desafios++
			if d.Desafios >= ciclos {
				d.Principal = lider.ID
				d.Desafios = 0
				break
			}
			if incumbente != nil {
				d.Principal = incumbente.ID
			}
		}
	}

	if d.Principal != "" {
		d.Papeis[d.Principal] = database.NginxPapelPrincipal
	}
	return d
}

func grupoDoServidor(siteID *uint) string {
	if siteID == nil {
		return semUnidade
	}
	return fmt.Sprintf("%d", *siteID)
}

type somaDeTrafego struct {
	ServerID string
	Total    int
}

func trafegoPorServidor(janela time.Duration) (map[string]int, error) {
	fora := map[string]int{}
	if database.DB == nil {
		return fora, nil
	}

	corte := time.Now().UTC().Add(-janela)
	var linhas []somaDeTrafego
	err := database.DB.Model(&database.MetricLoadBalancer{}).
		Select("COALESCE(server_id::text, '') AS server_id, SUM(requests_count) AS total").
		Where("timestamp >= ?", corte).
		Group("COALESCE(server_id::text, '')").
		Scan(&linhas).Error
	if err != nil {
		return nil, err
	}
	for _, linha := range linhas {
		if linha.ServerID != "" {
			fora[linha.ServerID] = linha.Total
		}
	}
	return fora, nil
}

func Eleger() {
	if database.DB == nil {
		return
	}

	var servidores []database.Server
	if err := database.DB.Order("name ASC").Find(&servidores).Error; err != nil {
		log.Printf("[Malha] erro ao listar servidores: %v", err)
		return
	}

	trafego, err := trafegoPorServidor(JanelaDeTrafego())
	if err != nil {
		log.Printf("[Malha] erro ao somar o tráfego do balanceador: %v", err)
		return
	}

	grupos := map[string][]Membro{}
	unidades := map[string]*uint{}
	nomes := map[string]string{}
	for _, s := range servidores {
		chave := grupoDoServidor(s.SiteID)
		unidades[chave] = s.SiteID
		nomes[s.ID] = s.Name
		grupos[chave] = append(grupos[chave], Membro{
			ID:          s.ID,
			Nome:        s.Name,
			Candidato:   s.NginxEstado == database.NginxCandidato || s.CollectNginx,
			Requisicoes: trafego[s.ID],
			PapelAtual:  s.NginxPapel,
		})
	}

	ciclos := ciclosParaTrocar()
	margem := margemDeTroca()
	for chave, membros := range grupos {
		aplicar(chave, unidades[chave], membros, nomes, trafego, ciclos, margem)
	}
}

func aplicar(grupo string, siteID *uint, membros []Membro, nomes map[string]string, trafego map[string]int, ciclos int, margem float64) {
	mu.Lock()
	d := decidir(membros, desafios[grupo], ciclos, margem)
	if d.Desafios == 0 {
		delete(desafios, grupo)
	} else {
		desafios[grupo] = d.Desafios
	}
	mu.Unlock()

	agora := time.Now().UTC()
	for _, m := range membros {
		papel := d.Papeis[m.ID]
		if papel == m.PapelAtual {
			continue
		}
		err := database.DB.Model(&database.Server{}).Where("id = ?", m.ID).Updates(map[string]any{
			"nginx_papel":       papel,
			"nginx_papel_desde": agora,
		}).Error
		if err != nil {
			log.Printf("[Malha] erro ao gravar o papel de %s: %v", m.Nome, err)
		}
	}

	if !d.Trocou() {
		return
	}
	anunciar(grupo, siteID, d, nomes, trafego)
}

func anunciar(grupo string, siteID *uint, d Decisao, nomes map[string]string, trafego map[string]int) {
	anterior := nomes[d.Anterior]
	novo := nomes[d.Principal]

	entrada := alert.Entrada{
		Key:      fmt.Sprintf("nginx_principal:%s:%s:%s", grupo, ladoDaChave(d.Anterior), ladoDaChave(d.Principal)),
		SiteID:   siteID,
		AlvoTipo: database.AlvoTipoServico,
		AlvoID:   "nginx:" + grupo,
		Metrica:  "papel_do_balanceador",
		Unidade:  "req",
	}

	switch {
	case d.Anterior == "":
		entrada.Severity = "info"
		entrada.AlvoNome = novo
		entrada.Text = fmt.Sprintf("[INFO] %s assumiu como balanceador principal: é quem está recebendo tráfego agora", novo)

	case d.Principal == "":
		entrada.Severity = "high"
		entrada.AlvoNome = anterior
		entrada.Text = fmt.Sprintf("[ALERTA] %s deixou de ser o balanceador principal e nenhum candidato assumiu: ninguém está provadamente recebendo tráfego", anterior)

	default:
		entrada.Severity = "high"
		entrada.AlvoNome = novo
		entrada.Text = fmt.Sprintf("[ALERTA] o balanceador principal passou de %s para %s: o tráfego migrou de máquina", anterior, novo)
	}

	if d.Principal != "" {
		id := d.Principal
		entrada.ServerID = &id
		if req, ok := trafego[d.Principal]; ok {
			valor := float64(req)
			entrada.Valor = &valor
		}
	} else if d.Anterior != "" {
		id := d.Anterior
		entrada.ServerID = &id
	}

	alert.Enqueue(entrada)
}

func ladoDaChave(id string) string {
	if id == "" {
		return "nenhum"
	}
	return id
}

func StartWorker(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = IntervaloDaEleicao()
	}
	safego.Run(ctx, "malha:eleicao", func(ctx context.Context) {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			Eleger()
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	})
}
