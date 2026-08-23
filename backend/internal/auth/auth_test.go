package auth

import "testing"

func TestAllows(t *testing.T) {
	cases := []struct {
		has, needs string
		want       bool
	}{
		{RoleAdmin, RoleAdmin, true},
		{RoleAdmin, RoleOperator, true},
		{RoleAdmin, RoleViewer, true},
		{RoleOperator, RoleViewer, true},
		{RoleOperator, RoleOperator, true},
		{RoleOperator, RoleAdmin, false},
		{RoleViewer, RoleOperator, false},
		{RoleViewer, RoleAdmin, false},
		{"inventado", RoleViewer, false},
		{RoleViewer, "inventado", false},
	}
	for _, c := range cases {
		if got := Allows(c.has, c.needs); got != c.want {
			t.Errorf("Allows(%q, %q) = %v, esperado %v", c.has, c.needs, got, c.want)
		}
	}
}

func TestValidRole(t *testing.T) {
	for _, r := range []string{RoleViewer, RoleOperator, RoleAdmin} {
		if !ValidRole(r) {
			t.Errorf("%q deveria ser válido", r)
		}
	}
	if ValidRole("root") {
		t.Error("papel inventado foi aceito")
	}
}

// Senha curta é o vetor mais comum de conta comprometida num painel interno.
func TestHashPasswordExigeTamanho(t *testing.T) {
	if _, err := HashPassword("curta"); err != ErrWeakPassword {
		t.Errorf("senha curta: erro = %v", err)
	}
	if _, err := HashPassword("          "); err != ErrWeakPassword {
		t.Error("senha só de espaços foi aceita")
	}

	hash, err := HashPassword("senha-bem-longa-123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "senha-bem-longa-123" {
		t.Fatal("a senha foi gravada em texto puro")
	}
}

func TestSessaoInvalidaNaoResolve(t *testing.T) {
	if _, ok := Lookup(""); ok {
		t.Error("token vazio virou sessão")
	}
	if _, ok := Lookup("nao-existe"); ok {
		t.Error("token inventado virou sessão")
	}
}

// Os dois testes abaixo passaram a exigir banco: a sessão deixou de viver num
// mapa em memória, e a autorização é relida a cada Lookup — sessão de usuário
// que não existe na tabela não resolve mais, por desenho.
func TestLogoutInvalidaSessao(t *testing.T) {
	user := setupSessaoDB(t)

	session, err := CreateSession(user.ID, user.Username, []Access{{SiteID: nil, Role: RoleOperator}})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, ok := Lookup(session.Token); !ok {
		t.Fatal("sessão recém-criada não resolve")
	}

	Logout(session.Token)
	if _, ok := Lookup(session.Token); ok {
		t.Fatal("sessão continuou válida depois do logout")
	}
}

// Trocar papel ou desativar alguém precisa valer na hora, não no fim da sessão.
func TestRevokeUserDerrubaTodasAsSessoes(t *testing.T) {
	user := setupSessaoDB(t)
	acc := []Access{{SiteID: nil, Role: RoleOperator}}

	primeira, err := CreateSession(user.ID, user.Username, acc)
	if err != nil {
		t.Fatalf("primeira sessão: %v", err)
	}
	segunda, err := CreateSession(user.ID, user.Username, acc)
	if err != nil {
		t.Fatalf("segunda sessão: %v", err)
	}

	RevokeUser(user.ID)

	if _, ok := Lookup(primeira.Token); ok {
		t.Error("primeira sessão sobreviveu")
	}
	if _, ok := Lookup(segunda.Token); ok {
		t.Error("segunda sessão sobreviveu")
	}
}

func site(id uint) *uint { return &id }

// O modelo copia o desenho do Zabbix: papel diz o que pode, concessão diz onde.
func TestRoleForSiteEEscopo(t *testing.T) {
	restrito := []Access{{SiteID: site(3), Role: RoleOperator}}
	global := []Access{{SiteID: nil, Role: RoleViewer}, {SiteID: site(3), Role: RoleOperator}}

	if HasGlobal(restrito) {
		t.Error("concessão só de unidade virou global")
	}
	if !HasGlobal(global) {
		t.Error("concessão global não foi reconhecida")
	}

	// Restrito: opera na unidade 3, não enxerga a 9 nem o escopo sem unidade.
	if got := RoleForSite(restrito, site(3)); got != RoleOperator {
		t.Errorf("papel na própria unidade = %q", got)
	}
	if CanSeeSite(restrito, site(9)) {
		t.Error("unidade alheia ficou visível")
	}
	if CanSeeSite(restrito, nil) {
		t.Error("escopo sem unidade (VPS/Dev) ficou visível para restrito")
	}

	// Global viewer + operator na 3: vence o maior papel por alvo.
	if got := RoleForSite(global, site(3)); got != RoleOperator {
		t.Errorf("papel combinado na unidade 3 = %q", got)
	}
	if got := RoleForSite(global, site(9)); got != RoleViewer {
		t.Errorf("papel global na unidade 9 = %q", got)
	}
	if got := RoleForSite(global, nil); got != RoleViewer {
		t.Errorf("papel no escopo sem unidade = %q", got)
	}
}

func TestMaxRoleEUsadoNoGateGrosso(t *testing.T) {
	if got := MaxRole([]Access{{SiteID: site(1), Role: RoleViewer}, {SiteID: site(2), Role: RoleAdmin}}); got != RoleAdmin {
		t.Errorf("MaxRole = %q", got)
	}
	if got := MaxRole(nil); got != "" {
		t.Errorf("MaxRole vazio = %q", got)
	}
}

func TestSiteIDsDeduplicaEIgnoraGlobais(t *testing.T) {
	ids := SiteIDs([]Access{
		{SiteID: site(2), Role: RoleViewer},
		{SiteID: site(2), Role: RoleOperator},
		{SiteID: nil, Role: RoleAdmin},
		{SiteID: site(7), Role: RoleViewer},
	})
	if len(ids) != 2 || ids[0] != 2 || ids[1] != 7 {
		t.Errorf("SiteIDs = %v", ids)
	}
}
