package ssh

import (
	"fmt"
	"time"
)

const janelaDoFormatoNginx = time.Minute

type vigiaDeFormatoNginx struct {
	host         string
	inicio       time.Time
	recebidas    int
	reconhecidas int
}

func novoVigiaDeFormatoNginx(host string, agora time.Time) *vigiaDeFormatoNginx {
	return &vigiaDeFormatoNginx{host: host, inicio: agora}
}

func (v *vigiaDeFormatoNginx) observar(reconhecida bool, agora time.Time) (string, bool) {
	v.recebidas++
	if reconhecida {
		v.reconhecidas++
	}
	if agora.Sub(v.inicio) < janelaDoFormatoNginx {
		return "", false
	}

	recebidas, reconhecidas := v.recebidas, v.reconhecidas
	v.inicio, v.recebidas, v.reconhecidas = agora, 0, 0
	if reconhecidas*100 >= recebidas {
		return "", false
	}
	return fmt.Sprintf("[Nginx] %s: %d linhas recebidas, %d reconhecidas; o access log não está no formato exigido (ver docs/operacao.md#formato-do-access-log-do-nginx)", v.host, recebidas, reconhecidas), true
}
