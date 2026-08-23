package rules

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/joaov/vd_stats/internal/database"
)

const (
	srvDuracao   = "00000000-0000-0000-0000-0000000000d1"
	prefixoRegra = "e9-teste-"
)

// setupMotorDB liga no Postgres de desenvolvimento. Sem DATABASE_URL o teste é
// pulado: a suíte precisa passar numa máquina sem banco.
//
// A cobertura é de integração porque o que se mede é a travessia inteira —
// evaluate lê métrica, decide, grava estado e o tick seguinte lê o que ficou
// gravado. A decisão isolada está coberta pelos testes puros de breachStart.
func setupMotorDB(t *testing.T) {
	t.Helper()

	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido; pulando teste do motor de regras")
	}
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			t.Skipf("banco indisponível: %v", err)
		}
	}
	limparMotor(t)
	t.Cleanup(func() { limparMotor(t) })

	srv := database.Server{
		ID: srvDuracao, Name: "host-duracao", HostIP: "10.93.0.1",
		Kind: "agent", ReportIntervalSec: 30,
	}
	if err := database.DB.Create(&srv).Error; err != nil {
		t.Fatalf("criar servidor: %v", err)
	}
}

func limparMotor(t *testing.T) {
	t.Helper()

	var regras []database.AlertRule
	database.DB.Where("name LIKE ?", prefixoRegra+"%").Find(&regras)
	for _, r := range regras {
		database.DB.Where("rule_id = ?", r.ID).Delete(&database.AlertState{})
	}
	database.DB.Where("name LIKE ?", prefixoRegra+"%").Delete(&database.AlertRule{})
	database.DB.Where("server_id = ?", srvDuracao).Delete(&database.MetricServer{})
	database.DB.Unscoped().Where("id = ?", srvDuracao).Delete(&database.Server{})
}

// criarRegra devolve a regra já gravada, para o teste conhecer o id e poder ler
// o estado dela depois.
func criarRegra(t *testing.T, nome string, duracaoSec int) database.AlertRule {
	t.Helper()

	regra := database.AlertRule{
		Name: prefixoRegra + nome, Target: srvDuracao, Metric: "cpu",
		Operator: ">", Threshold: 50, Enabled: true,
		Severity: SeverityCritical, ForDurationSec: duracaoSec,
	}
	if err := database.DB.Create(&regra).Error; err != nil {
		t.Fatalf("criar regra: %v", err)
	}
	return regra
}

// amostra grava uma leitura de CPU mais recente que todas as anteriores.
func amostra(t *testing.T, cpu float64, idade time.Duration) {
	t.Helper()

	m := database.MetricServer{
		ServerID: srvDuracao, CPUUsagePercent: cpu,
		Timestamp: time.Now().UTC().Add(-idade),
	}
	if err := database.DB.Create(&m).Error; err != nil {
		t.Fatalf("gravar métrica: %v", err)
	}
}

func estadoDe(t *testing.T, regra database.AlertRule) database.AlertState {
	t.Helper()

	var st database.AlertState
	if err := database.DB.Where("key = ?", stateKey(regra.ID, srvDuracao)).
		Take(&st).Error; err != nil {
		t.Fatalf("ler estado da regra %d: %v", regra.ID, err)
	}
	return st
}

// capturarLog redireciona o log do processo. Com o Telegram desligado — que é o
// estado num teste —, alert.Send escreve a mensagem no log, e é assim que o
// teste observa se o aviso saiu.
func capturarLog(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	original := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(original) })
	return &buf
}

// avisos conta as linhas do log que citam esta regra. Filtra pelo nome porque
// vários pacotes de teste compartilham o mesmo banco, e outras regras podem
// estar disparando no mesmo evaluate.
func avisos(buf *bytes.Buffer, nome, trecho string) int {
	n := 0
	for _, linha := range strings.Split(buf.String(), "\n") {
		if strings.Contains(linha, prefixoRegra+nome) && strings.Contains(linha, trecho) {
			n++
		}
	}
	return n
}

// Uma regra com duração não dispara na primeira amostra. É o item E9 inteiro:
// antes, um pico único de compilação ou de backup virava incidente, e o operador
// aprendia a ignorar o canal.
func TestRegraComDuracaoSeguraODisparo(t *testing.T) {
	setupMotorDB(t)
	regra := criarRegra(t, "segura", 3600)
	amostra(t, 90, 0)

	buf := capturarLog(t)
	evaluate()

	if n := avisos(buf, "segura", "Regra"); n != 0 {
		t.Errorf("a regra disparou %d vez(es) na primeira amostra, com duração de 1h exigida", n)
	}

	st := estadoDe(t, regra)
	if st.FirstBreachAt.IsZero() {
		t.Error("a contagem não começou: first_breach_at ficou zerado")
	}
	if st.Active {
		t.Error("o alvo ficou marcado como anunciado sem nenhum aviso ter saído")
	}
}

// O controle que impede a correção de virar "nunca dispara": duração zero é o
// valor de toda regra já cadastrada, e precisa continuar alertando na hora.
func TestRegraSemDuracaoDisparaNaPrimeiraAmostra(t *testing.T) {
	setupMotorDB(t)
	regra := criarRegra(t, "imediata", 0)
	amostra(t, 90, 0)

	buf := capturarLog(t)
	evaluate()

	if n := avisos(buf, "imediata", "Regra"); n != 1 {
		t.Fatalf("avisos = %d, esperado 1: duração zero tem que disparar na primeira amostra", n)
	}
	if st := estadoDe(t, regra); !st.Active {
		t.Error("o alerta saiu mas o alvo não ficou marcado como anunciado")
	}
}

// A travessia completa do caso "acima, abaixo": a amostra dentro do limite
// precisa zerar a contagem no banco, senão a violação seguinte herda o tempo já
// acumulado e "por 5 minutos seguidos" vira "5 minutos somados no dia".
func TestAmostraDentroDoLimiteZeraAContagemGravada(t *testing.T) {
	setupMotorDB(t)
	regra := criarRegra(t, "zera", 3600)

	amostra(t, 90, time.Second)
	evaluate()
	if estadoDe(t, regra).FirstBreachAt.IsZero() {
		t.Fatal("a contagem não chegou a começar; o teste não mediria nada")
	}

	amostra(t, 10, 0)
	evaluate()

	if got := estadoDe(t, regra).FirstBreachAt; !got.IsZero() {
		t.Errorf("first_breach_at = %v, esperado zerado pela amostra dentro do limite", got)
	}
}

// A recuperação sai uma vez, na transição, e não a cada tick com o host normal.
func TestRecuperacaoSaiUmaVezSo(t *testing.T) {
	setupMotorDB(t)
	criarRegra(t, "recupera", 0)

	amostra(t, 90, time.Second)
	evaluate()

	amostra(t, 10, 0)
	buf := capturarLog(t)
	evaluate()

	if n := avisos(buf, "recupera", "Recuperado"); n != 1 {
		t.Fatalf("avisos de recuperação = %d, esperado 1", n)
	}

	buf.Reset()
	evaluate()
	if n := avisos(buf, "recupera", "Recuperado"); n != 0 {
		t.Errorf("a recuperação foi anunciada %d vez(es) a mais, com o host já normal", n)
	}
}

// Recuperação só depois de anúncio: um problema que nunca foi comunicado não
// pode gerar um "voltou ao normal" que o operador não entende. É também o que
// contém o flapping — o anúncio passa pelo cooldown, então o par
// alerta/recuperação não acontece mais de uma vez por janela.
func TestRecuperacaoNaoSaiSemAlertaAnterior(t *testing.T) {
	setupMotorDB(t)
	criarRegra(t, "silenciosa", 3600)

	amostra(t, 90, time.Second)
	evaluate()

	amostra(t, 10, 0)
	buf := capturarLog(t)
	evaluate()

	if n := avisos(buf, "silenciosa", "Recuperado"); n != 0 {
		t.Errorf("saiu recuperação de um problema que nunca foi anunciado (%d aviso(s))", n)
	}
}
