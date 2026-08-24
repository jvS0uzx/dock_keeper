package ssh

import "testing"

func TestValidContainerName(t *testing.T) {
	validos := []string{"nginx", "meu_app", "app.1", "web-01", "A1", "a"}
	for _, nome := range validos {
		if !validContainerName.MatchString(nome) {
			t.Errorf("nome legítimo recusado: %q", nome)
		}
	}

	// O hífen inicial é o caso do S9: "-f" e "--rm" passariam a ser lidos pelo
	// docker como flag, não como o container alvo.
	invalidos := []string{"-f", "--rm", "-", "_app", ".oculto", "", "a b", "a;id", "a$(id)", "a|b", "../etc"}
	for _, nome := range invalidos {
		if validContainerName.MatchString(nome) {
			t.Errorf("nome perigoso aceito: %q", nome)
		}
	}
}
