package ssh

import "testing"

// IsAllowedAction é consultada pelo handler para decidir o que vai escrito na
// coluna de ação da auditoria. Se ela afrouxar, o corpo da requisição passa a
// escolher o conteúdo da tabela — por isso a lista fechada tem teste próprio.
func TestIsAllowedActionSoAceitaOsTresVerbos(t *testing.T) {
	casos := []struct {
		action string
		aceita bool
	}{
		{"start", true},
		{"stop", true},
		{"restart", true},
		{"", false},
		{"rm", false},
		{"rm -rf /", false},
		{"exec", false},
		{"kill", false},
		{"Stop", false},
		{"stop; id", false},
	}
	for _, c := range casos {
		if got := IsAllowedAction(c.action); got != c.aceita {
			t.Errorf("IsAllowedAction(%q) = %v, esperado %v", c.action, got, c.aceita)
		}
	}
}

// RunContainerAction precisa continuar recusando por conta própria: a
// conferência no handler é a primeira camada, não a única. Se ela sumir do
// pacote, uma chamada direta de outro caminho passaria batido.
func TestRunContainerActionRecusaAntesDeAbrirSessao(t *testing.T) {
	alvo := Target{ID: "x", Name: "n", Host: "203.0.113.1", User: "root", Port: 22}

	if _, err := RunContainerAction(alvo, "rm -rf /", "nginx"); err == nil {
		t.Error("ação fora da allowlist foi aceita")
	}
	if _, err := RunContainerAction(alvo, "stop", "-f"); err == nil {
		t.Error("nome de container começando por hífen foi aceito")
	}
}
