package ssh

import (
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Padrões de Debian e Ubuntu, que é onde o projeto nasceu. Em RHEL o auth.log
// se chama /var/log/secure, e o caminho cravado deixava a tela de Segurança
// vazia sem nenhum erro aparecer — o pior modo de falha possível, porque parece
// "nenhum evento" em vez de "não consegui ler".
const (
	defaultAuthLogPath  = "/var/log/auth.log"
	defaultNginxLogPath = "/var/log/nginx/access.log"

	// Segundos entre amostras do script de coleta por SSH. Dois é o ritmo que a
	// tela de tempo real espera; subir alivia CPU do host monitorado ao custo de
	// granularidade.
	defaultCollectInterval = 2
)

// safeRemotePath aceita só caminho absoluto sem metacaractere de shell.
//
// O valor vem da configuração do painel, não de requisição, mas ele é
// interpolado num comando que roda como root na máquina remota: um operador que
// cole um caminho com aspas ou ponto-e-vírgula por engano não pode transformar
// configuração em execução de comando.
var safeRemotePath = regexp.MustCompile(`^/[A-Za-z0-9._/-]+$`)

// AuthLogPath é o arquivo de log de autenticação do host monitorado.
func AuthLogPath() string {
	return remotePathFromEnv("SSH_AUTH_LOG_PATH", defaultAuthLogPath)
}

// NginxLogPath é o access log do Nginx no host monitorado.
func NginxLogPath() string {
	return remotePathFromEnv("SSH_NGINX_LOG_PATH", defaultNginxLogPath)
}

func remotePathFromEnv(key, def string) string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	if !safeRemotePath.MatchString(raw) {
		log.Printf("[SSH] %s=%q recusado: use caminho absoluto sem espaço nem metacaractere. Usando %s",
			key, raw, def)
		return def
	}
	return raw
}

// CollectIntervalSec é o intervalo do laço do script de coleta.
func CollectIntervalSec() int {
	raw := strings.TrimSpace(os.Getenv("SSH_COLLECT_INTERVAL"))
	if raw == "" {
		return defaultCollectInterval
	}

	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		log.Printf("[SSH] SSH_COLLECT_INTERVAL=%q inválido, usando %d segundos", raw, defaultCollectInterval)
		return defaultCollectInterval
	}
	return n
}

// scriptPrelude monta as atribuições que o Go injeta no topo do script.
//
// Os scripts leem essas variáveis com fallback embutido (`${VD_INTERVAL:-2}`),
// então um script executado à mão, fora do painel, continua funcionando.
func scriptPrelude() string {
	var b strings.Builder
	b.WriteString("VD_INTERVAL=" + strconv.Itoa(CollectIntervalSec()) + "\n")
	b.WriteString("VD_NGINX_LOG=" + NginxLogPath() + "\n")
	return b.String()
}
