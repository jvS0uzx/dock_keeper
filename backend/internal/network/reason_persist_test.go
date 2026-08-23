package network

import (
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/joaov/vd_stats/internal/database"
)

const dominioDeTeste = "127.0.0.1"

func setupDominioDB(t *testing.T) {
	t.Helper()

	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido; pulando teste de persistência do motivo")
	}
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			t.Skipf("banco indisponível: %v", err)
		}
	}
	limparDominioDeTeste(t)
	t.Cleanup(func() { limparDominioDeTeste(t) })
}

func limparDominioDeTeste(t *testing.T) {
	t.Helper()
	database.DB.Where("name = ?", dominioDeTeste).Delete(&database.Domain{})
}

func lerDominio(t *testing.T, id uint) database.Domain {
	t.Helper()

	var d database.Domain
	if err := database.DB.First(&d, id).Error; err != nil {
		t.Fatalf("reler o domínio %d: %v", id, err)
	}
	return d
}

// servidorTLSConfiavel sobe um TLS local e aponta a verificação para ele, com o
// certificado dele nas raízes. É o que permite exercitar CheckAndStore no
// caminho do certificado VÁLIDO sem sair para a internet — e o caminho válido é
// justamente onde mora o defeito de não limpar o motivo anterior.
func servidorTLSConfiavel(t *testing.T) {
	t.Helper()

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(ts.Close)

	_, porta, err := net.SplitHostPort(ts.Listener.Addr().String())
	if err != nil {
		t.Fatalf("porta do servidor de teste: %v", err)
	}

	caminho := filepath.Join(t.TempDir(), "ca.pem")
	bloco := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ts.Certificate().Raw})
	if err := os.WriteFile(caminho, bloco, 0o600); err != nil {
		t.Fatalf("gravar CA de teste: %v", err)
	}

	t.Setenv("SSL_CHECK_PORT", porta)
	t.Setenv("SSL_EXTRA_CA", caminho)
}

// O motivo precisa chegar ao banco: enquanto a coluna não existia, a
// classificação morria em SSLInfo e a tela só recebia a frase de error_msg.
func TestMotivoDaInvalidezEPersistido(t *testing.T) {
	setupDominioDB(t)

	// Sem SSL_EXTRA_CA o certificado do servidor local não é confiável, então a
	// verificação classifica o motivo em vez de aprovar.
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()
	_, porta, err := net.SplitHostPort(ts.Listener.Addr().String())
	if err != nil {
		t.Fatalf("porta do servidor de teste: %v", err)
	}
	t.Setenv("SSL_CHECK_PORT", porta)

	d := database.Domain{Name: dominioDeTeste}
	if err := database.DB.Create(&d).Error; err != nil {
		t.Fatalf("criar domínio: %v", err)
	}

	CheckAndStore(d)

	gravado := lerDominio(t, d.ID)
	if gravado.Valid {
		t.Fatalf("o certificado não confiável saiu como válido; o teste não mede nada")
	}
	if gravado.InvalidReason == "" {
		t.Error("invalid_reason ficou vazio para certificado inválido: o motivo não chegou ao banco")
	}
}

// O defeito que este teste existe para impedir: gravar o motivo só quando há
// problema deixa o domínio verde exibindo, para sempre, a causa da falha
// anterior. A renovação de certificado é justamente o momento em que ninguém
// olha de novo.
func TestRenovacaoLimpaOMotivoAnterior(t *testing.T) {
	setupDominioDB(t)
	servidorTLSConfiavel(t)

	d := database.Domain{Name: dominioDeTeste}
	if err := database.DB.Create(&d).Error; err != nil {
		t.Fatalf("criar domínio: %v", err)
	}

	// Estado sujo, como o de um domínio que falhou num ciclo anterior.
	if err := database.DB.Model(&database.Domain{}).Where("id = ?", d.ID).
		Updates(map[string]any{
			"valid":          false,
			"invalid_reason": ReasonAutoassinado,
			"error_msg":      "Certificado autoassinado: o emissor é o próprio titular",
		}).Error; err != nil {
		t.Fatalf("sujar o estado do domínio: %v", err)
	}

	CheckAndStore(d)

	gravado := lerDominio(t, d.ID)
	if !gravado.Valid {
		t.Fatalf("o certificado local não foi aceito (%s); o teste não mede a limpeza", gravado.ErrorMsg)
	}
	if gravado.InvalidReason != "" {
		t.Errorf("invalid_reason = %q depois da renovação, esperado vazio: a tela mostraria o motivo antigo num domínio verde",
			gravado.InvalidReason)
	}
	if gravado.ErrorMsg != "" {
		t.Errorf("error_msg = %q depois da renovação, esperado vazio", gravado.ErrorMsg)
	}
}
