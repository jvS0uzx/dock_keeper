package ssh

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func alvoDeBancosContado(t *testing.T, serverID string, ligada bool, chamadas *atomic.Int32) Target {
	t.Helper()

	alvo := alvoComServidor(t, func(string) respostaExec {
		chamadas.Add(1)
		return respostaExec{stdout: `{"descoberta_host_ok":true,"descoberta_container_ok":true,"instancias":[]}` + "\n", consomeStdin: true}
	})
	alvo.ID = serverID
	alvo.CollectBancos = ligada
	return alvo
}

func esperarRetorno(t *testing.T, rodar func(context.Context)) bool {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pronto := make(chan struct{})
	go func() {
		rodar(ctx)
		close(pronto)
	}()

	select {
	case <-pronto:
		return ctx.Err() == nil
	case <-time.After(5 * time.Second):
		t.Fatal("a sonda não devolveu o controle nem depois de cancelada")
		return false
	}
}

func TestSondaPostgresDesligadaNaoRodaEApagaOInventario(t *testing.T) {
	srv := servidorDeTeste(t)

	agora := time.Now().UTC()
	if err := gravarSondaPostgres(srv.ID, sondaPostgresDeTeste(true, true, instanciaDeTeste(5432, false)), agora); err != nil {
		t.Fatalf("semear inventário: %v", err)
	}
	if len(instanciasGravadas(t, srv.ID)) != 1 {
		t.Fatal("inventário não foi semeado")
	}

	var chamadas atomic.Int32
	alvo := alvoDeBancosContado(t, srv.ID, false, &chamadas)

	if !esperarRetorno(t, func(ctx context.Context) { SondarPostgresPeriodicamente(ctx, alvo) }) {
		t.Error("a sonda desligada ficou rodando até o contexto expirar, esperado retorno imediato")
	}
	if n := chamadas.Load(); n != 0 {
		t.Errorf("a sonda desligada abriu %d sessão(ões) SSH, esperado nenhuma", n)
	}
	if linhas := instanciasGravadas(t, srv.ID); len(linhas) != 0 {
		t.Errorf("instâncias = %+v, esperado inventário apagado com a sonda desligada", linhas)
	}

	var bases int64
	database.DB.Model(&database.PostgresBase{}).
		Joins("JOIN postgres_instancias ON postgres_instancias.id = postgres_bases.instancia_id").
		Where("postgres_instancias.server_id = ?", srv.ID).Count(&bases)
	if bases != 0 {
		t.Errorf("bases = %d, esperado nenhuma depois de apagar o inventário", bases)
	}
}

func TestSondaPostgresLigadaContinuaRodando(t *testing.T) {
	srv := servidorDeTeste(t)

	var chamadas atomic.Int32
	alvo := alvoDeBancosContado(t, srv.ID, true, &chamadas)

	if esperarRetorno(t, func(ctx context.Context) { SondarPostgresPeriodicamente(ctx, alvo) }) {
		t.Error("a sonda ligada devolveu o controle antes do contexto acabar")
	}
	if chamadas.Load() == 0 {
		t.Error("a sonda ligada não abriu sessão SSH nenhuma")
	}
}

func TestSondaDesligadaNaoApagaInventarioDeOutroServidor(t *testing.T) {
	srv := servidorDeTeste(t)
	outro := database.Server{Name: nomeServidorDeTeste + "-outro", HostIP: "203.0.113.98"}
	if err := database.DB.Create(&outro).Error; err != nil {
		t.Fatalf("criar outro servidor: %v", err)
	}
	t.Cleanup(func() { limpaServidorDeTeste(t, outro.ID) })

	agora := time.Now().UTC()
	if err := gravarSondaPostgres(outro.ID, sondaPostgresDeTeste(true, true, instanciaDeTeste(5432, false)), agora); err != nil {
		t.Fatalf("semear inventário do outro: %v", err)
	}

	if !sondaDeBancosDesligada("Postgres", Target{ID: srv.ID, Host: "desligado"}) {
		t.Fatal("Target com CollectBancos=false não foi tratado como desligado")
	}
	if len(instanciasGravadas(t, outro.ID)) != 1 {
		t.Error("desligar a sonda de um servidor apagou o inventário de outro")
	}
}
