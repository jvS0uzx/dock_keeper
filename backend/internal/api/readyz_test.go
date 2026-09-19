package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func pedirReadyz(t *testing.T) (int, map[string]any) {
	t.Helper()

	rec := httptest.NewRecorder()
	Routes(testConfig()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	var corpo map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &corpo)
	return rec.Code, corpo
}

func trocarBanco(t *testing.T, db *gorm.DB) {
	t.Helper()

	original := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = original })
}

func TestReadyzSemBancoResponde503(t *testing.T) {
	trocarBanco(t, nil)

	code, _ := pedirReadyz(t)
	if code != http.StatusServiceUnavailable {
		t.Errorf("/readyz com banco nulo: status %d, esperado 503", code)
	}
}

func TestReadyzComBancoRespondendo200(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido; pulando /readyz com banco")
	}
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			t.Skipf("banco indisponível: %v", err)
		}
	}

	code, corpo := pedirReadyz(t)
	if code != http.StatusOK || corpo["status"] != "ok" {
		t.Errorf("/readyz com banco no ar: status %d corpo %v, esperado 200 {\"status\":\"ok\"}", code, corpo)
	}
}

func TestReadyzComBancoForaDoArResponde503(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL não definido; pulando /readyz com banco fechado")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Skipf("banco indisponível: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql.DB: %v", err)
	}
	_ = sqlDB.Close()
	trocarBanco(t, db)

	code, _ := pedirReadyz(t)
	if code != http.StatusServiceUnavailable {
		t.Errorf("/readyz com a conexão fechada: status %d, esperado 503", code)
	}
}

func TestHealthzNaoTocaNoBanco(t *testing.T) {
	trocarBanco(t, nil)

	rec := httptest.NewRecorder()
	Routes(testConfig()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("/healthz com banco nulo: status %d, esperado 200", rec.Code)
	}
}

func TestReadyzMostraOEstadoDoCanalDeAlerta(t *testing.T) {
	trocarBanco(t, nil)

	_, corpo := pedirReadyz(t)
	if corpo["alertas"] == nil {
		t.Errorf("/readyz não informa o estado do canal de alerta: %v", corpo)
	}
}

func pedirComOrigem(t *testing.T, caminho, origem string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, caminho, nil)
	if origem != "" {
		req.Header.Set("Origin", origem)
	}
	rec := httptest.NewRecorder()
	Routes(testConfig()).ServeHTTP(rec, req)
	return rec
}

func TestApiReadyzRespondeIgualAoReadyz(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido; pulando /api/readyz com banco")
	}
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			t.Skipf("banco indisponível: %v", err)
		}
	}

	codeAntigo, corpoAntigo := pedirReadyz(t)
	rec := pedirComOrigem(t, "/api/readyz", "")
	if rec.Code != codeAntigo {
		t.Fatalf("/api/readyz: status %d, /readyz devolveu %d", rec.Code, codeAntigo)
	}

	var corpoNovo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &corpoNovo); err != nil {
		t.Fatalf("corpo do /api/readyz: %v", err)
	}
	if corpoNovo["status"] != corpoAntigo["status"] {
		t.Errorf("status = %v, esperado %v", corpoNovo["status"], corpoAntigo["status"])
	}
	if _, tem := corpoNovo["alertas"]; !tem {
		t.Errorf("/api/readyz não trouxe o estado do canal de alerta: %v", corpoNovo)
	}
}

func TestApiReadyzMandaCabecalhoDeOrigem(t *testing.T) {
	const permitida = "https://painel.exemplo.com"

	rec := pedirComOrigem(t, "/api/readyz", permitida)
	if rec.Header().Get("Access-Control-Allow-Origin") != permitida {
		t.Errorf("/api/readyz devolveu origem %q, esperado %q; sem isso o navegador barra a faixa de degradação",
			rec.Header().Get("Access-Control-Allow-Origin"), permitida)
	}

	deFora := pedirComOrigem(t, "/api/readyz", "https://site-de-fora.exemplo")
	if deFora.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("origem fora da allowlist recebeu liberação de CORS")
	}
}

func TestApiReadyzSemBancoResponde503(t *testing.T) {
	trocarBanco(t, nil)

	rec := pedirComOrigem(t, "/api/readyz", "https://painel.exemplo.com")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("/api/readyz com banco nulo: status %d, esperado 503", rec.Code)
	}
}
