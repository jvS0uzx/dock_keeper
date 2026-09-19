package scripts

import _ "embed"

//go:embed stream_metrics.sh
var StreamMetrics string

//go:embed stream_nginx.sh
var StreamNginx string
