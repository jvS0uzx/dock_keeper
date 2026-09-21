package database

import "time"

const MinLiveWindow = 30 * time.Second

func LiveWindowFor(intervalSec int) time.Duration {
	if intervalSec <= 0 {
		return MinLiveWindow
	}
	if w := time.Duration(intervalSec) * 3 * time.Second; w > MinLiveWindow {
		return w
	}
	return MinLiveWindow
}

const ConsultaUltimasMetricas = `
	SELECT m.*
	FROM servers s
	JOIN LATERAL (
		SELECT *
		FROM metric_servers
		WHERE server_id = s.id
		  AND timestamp >= NOW() - make_interval(secs => GREATEST(COALESCE(s.report_interval_sec, 0) * 3, 30))
		ORDER BY timestamp DESC
		LIMIT 1
	) m ON TRUE
	WHERE s.deleted_at IS NULL
`
