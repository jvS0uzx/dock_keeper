package ssh

import (
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
)

const (
	defaultAuthLogPath  = "/var/log/auth.log"
	defaultNginxLogPath = "/var/log/nginx/access.log"

	defaultCollectInterval = 2
)

var safeRemotePath = regexp.MustCompile(`^/[A-Za-z0-9._/-]+$`)

func AuthLogPath() string {
	return remotePathFromEnv("SSH_AUTH_LOG_PATH", defaultAuthLogPath)
}

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

const sudoPrefix = "sudo -n "

func useSudo(t Target) bool {
	on, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv("SSH_USE_SUDO")))
	return on && t.User != "" && t.User != "root"
}

func scriptPrelude(t Target) string {
	var b strings.Builder
	b.WriteString("DOCKKEEPER_INTERVAL=" + strconv.Itoa(CollectIntervalSec()) + "\n")
	b.WriteString("DOCKKEEPER_NGINX_LOG=" + NginxLogPath() + "\n")
	if useSudo(t) {
		b.WriteString("DOCKKEEPER_TAIL=\"" + sudoPrefix + "/usr/bin/tail\"\n")
	}
	return b.String()
}
