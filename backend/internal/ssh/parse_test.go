package ssh

import "testing"

func TestParseDockerSize(t *testing.T) {
	cases := map[string]int64{
		"":           0,
		"512B":       512,
		"1.5MiB":     1_572_864,
		" 2GiB ":     2_147_483_648,
		"3kB":        3072,
		"1TiB":       1 << 40,
		"sem-numero": 0,
	}
	for input, want := range cases {
		if got := parseDockerSize(input); got != want {
			t.Errorf("parseDockerSize(%q) = %d, esperado %d", input, got, want)
		}
	}
}

func TestParsePercent(t *testing.T) {
	cases := map[string]float64{"12.34%": 12.34, " 0.00% ": 0, "": 0, "abc": 0}
	for input, want := range cases {
		if got := parsePercent(input); got != want {
			t.Errorf("parsePercent(%q) = %v, esperado %v", input, got, want)
		}
	}
}

func TestParseNginxLine(t *testing.T) {
	t.Run("upstream com erro", func(t *testing.T) {
		key, ok := parseNginxLine(`10.0.0.9 - app.exemplo.com to: 10.0.0.2:8080: GET /api 502 120`)
		if !ok {
			t.Fatal("linha válida foi descartada")
		}
		if key.ServerName != "app.exemplo.com" || key.Upstream != "10.0.0.2:8080" || key.Status != "502" {
			t.Fatalf("key = %+v", key)
		}
	})

	t.Run("sem upstream vira cache local", func(t *testing.T) {
		key, ok := parseNginxLine(`10.0.0.9 - app.exemplo.com to: -: GET / 200 10`)
		if !ok {
			t.Fatal("linha válida foi descartada")
		}
		if key.Upstream != "Local (Nginx/Cache)" || key.Status != "200" {
			t.Fatalf("key = %+v", key)
		}
	})

	t.Run("linha sem marcador", func(t *testing.T) {
		if _, ok := parseNginxLine("linha qualquer do log"); ok {
			t.Fatal("linha sem ' to: ' foi aceita")
		}
	})
}

func TestParseSSLine(t *testing.T) {
	line := `tcp   LISTEN 0      4096   0.0.0.0:22    0.0.0.0:*    users:(("sshd",pid=812,fd=3))`
	info, ok := parseSSLine(line)
	if !ok {
		t.Fatal("linha válida foi descartada")
	}
	if info.Protocol != "tcp" || info.State != "LISTEN" || info.Port != "22" || info.Process != "sshd" {
		t.Fatalf("info = %+v", info)
	}

	if _, ok := parseSSLine("tcp LISTEN 0"); ok {
		t.Fatal("linha truncada foi aceita")
	}
}

func TestTargetAddrUsaPortaConfigurada(t *testing.T) {
	if got := (Target{Host: "10.0.0.1", Port: 2222}).addr(); got != "10.0.0.1:2222" {
		t.Errorf("addr = %q", got)
	}
	if got := (Target{Host: "10.0.0.1"}).addr(); got != "10.0.0.1:22" {
		t.Errorf("addr sem porta = %q, esperado a porta padrão", got)
	}
}
