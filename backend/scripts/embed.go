// Package scripts embute os shell scripts de coleta no binário. Antes eles
// eram lidos com os.ReadFile("scripts/...") em runtime, o que amarrava o
// serviço ao diretório de trabalho e quebrava a coleta se o deploy copiasse só
// o executável.
package scripts

import _ "embed"

//go:embed stream_metrics.sh
var StreamMetrics string

//go:embed stream_nginx.sh
var StreamNginx string
