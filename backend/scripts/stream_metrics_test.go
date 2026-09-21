package scripts

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"text/template"
	"time"
)

type containerFalso struct {
	ID       string
	Names    string
	State    string
	Status   string
	CPUPerc  string
	MemUsage string
	labels   map[string]string
}

func (c containerFalso) Label(chave string) string { return c.labels[chave] }

var containerComAspas = containerFalso{
	ID:       "abc123",
	Names:    "api",
	State:    "running",
	Status:   `Up 2 hours (health: "starting")`,
	CPUPerc:  "1.50%",
	MemUsage: "10MiB / 1GiB",
	labels:   map[string]string{"com.docker.compose.project": `loja "beta"`},
}

func TestAjudanteDockerFalso(t *testing.T) {
	if os.Getenv("DK_DOCKER_FALSO") != "1" {
		return
	}

	args := os.Args
	for i, a := range args {
		if a == "--" {
			args = args[i+1:]
			break
		}
	}
	var formato string
	for i, a := range args {
		if a == "--format" && i+1 < len(args) {
			formato = args[i+1]
		}
	}

	tpl := template.Must(template.New("docker").Funcs(template.FuncMap{
		"json": func(v any) string {
			b, _ := json.Marshal(v)
			return string(b)
		},
	}).Parse(formato))
	if err := tpl.Execute(os.Stdout, containerComAspas); err != nil {
		os.Exit(2)
	}
	fmt.Println()
	os.Exit(0)
}

type saidaDoScript struct {
	t      *testing.T
	linhas chan string
}

func (s saidaDoScript) proxima() string {
	s.t.Helper()
	select {
	case l, ok := <-s.linhas:
		if !ok {
			s.t.Fatal("o script terminou sem emitir a linha esperada")
		}
		return l
	case <-time.After(15 * time.Second):
		s.t.Fatal("o script não emitiu nenhuma linha em 15 s")
		return ""
	}
}

func iniciarScript(t *testing.T, prelude string) saidaDoScript {
	t.Helper()

	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash indisponível")
	}

	dir := t.TempDir()
	falso := "#!/bin/sh\nexec \"" + os.Args[0] + "\" -test.run='^TestAjudanteDockerFalso$' -- \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(falso), 0o755); err != nil {
		t.Fatalf("criar docker falso: %v", err)
	}

	cmd := exec.Command(bash, "-s")
	cmd.Env = append(os.Environ(),
		"PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"DK_DOCKER_FALSO=1",
	)
	cmd.Stdin = strings.NewReader(prelude + StreamMetrics)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout do script: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("iniciar o script: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	linhas := make(chan string, 16)
	go func() {
		defer close(linhas)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			linhas <- scanner.Text()
		}
	}()
	return saidaDoScript{t: t, linhas: linhas}
}

func primeiraLinhaDoScript(t *testing.T) string {
	t.Helper()
	return iniciarScript(t, "DOCKKEEPER_INTERVAL=60\n").proxima()
}

func TestStreamMetricsEscapaAspasDoContainer(t *testing.T) {
	linha := primeiraLinhaDoScript(t)

	var payload struct {
		PS []struct {
			DockerID string `json:"docker_id"`
			Name     string `json:"name"`
			Project  string `json:"project"`
			State    string `json:"state"`
			Status   string `json:"status"`
		} `json:"ps"`
		Stats []struct {
			DockerID   string `json:"docker_id"`
			CPUPercent string `json:"cpu_percent"`
			MemUsage   string `json:"mem_usage"`
		} `json:"stats"`
	}
	if err := json.Unmarshal([]byte(linha), &payload); err != nil {
		t.Fatalf("a linha do script não é JSON válido com aspas no status do container: %v\n%s", err, linha)
	}

	if len(payload.PS) != 1 || len(payload.Stats) != 1 {
		t.Fatalf("esperado 1 container em ps e em stats, veio ps=%d stats=%d", len(payload.PS), len(payload.Stats))
	}
	ps := payload.PS[0]
	if ps.Status != containerComAspas.Status {
		t.Errorf("status voltou %q, esperado %q", ps.Status, containerComAspas.Status)
	}
	if ps.Project != `loja "beta"` {
		t.Errorf("projeto voltou %q, esperado %q", ps.Project, `loja "beta"`)
	}
	if ps.DockerID != "abc123" || ps.Name != "api" || ps.State != "running" {
		t.Errorf("campos de ps divergentes: %+v", ps)
	}
	st := payload.Stats[0]
	if st.DockerID != "abc123" || st.CPUPercent != "1.50%" || st.MemUsage != "10MiB / 1GiB" {
		t.Errorf("campos de stats divergentes: %+v", st)
	}
}

func escreverNetDev(t *testing.T, path string, virtualRx, virtualTx, ethRx, ethTx int64) {
	t.Helper()

	conteudo := "Inter-|   Receive                                                |  Transmit\n" +
		" face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed\n"
	for _, virtual := range []string{"lo", "veth123", "docker0", "br-abc", "virbr0"} {
		conteudo += fmt.Sprintf("%8s: %d 0 0 0 0 0 0 0 %d 0 0 0 0 0 0 0\n", virtual, virtualRx, virtualTx)
	}
	conteudo += fmt.Sprintf("  eth0: %d 0 0 0 0 0 0 0 %d 0 0 0 0 0 0 0\n", ethRx, ethTx)
	if err := os.WriteFile(path, []byte(conteudo), 0o644); err != nil {
		t.Fatalf("escrever net/dev falso: %v", err)
	}
}

type amostraDeRede struct {
	NetRx *float64 `json:"net_rx_bps"`
	NetTx *float64 `json:"net_tx_bps"`
}

func lerRede(t *testing.T, linha string) amostraDeRede {
	t.Helper()
	var a amostraDeRede
	if err := json.Unmarshal([]byte(linha), &a); err != nil {
		t.Fatalf("linha do script não é JSON válido: %v\n%s", err, linha)
	}
	return a
}

func TestStreamMetricsEmiteTaxaDeRedeSemContarInterfacesVirtuais(t *testing.T) {
	netDev := filepath.Join(t.TempDir(), "net_dev")
	escreverNetDev(t, netDev, 1000, 500, 5000, 7000)
	saida := iniciarScript(t, "DOCKKEEPER_INTERVAL=1\nDOCKKEEPER_NET_DEV="+netDev+"\n")

	if a := lerRede(t, saida.proxima()); a.NetRx != nil || a.NetTx != nil {
		t.Errorf("a primeira amostra não tem leitura anterior e trouxe rede (%v, %v)", a.NetRx, a.NetTx)
	}

	escreverNetDev(t, netDev, 1000+999999, 500+999999, 5000+3000, 7000+1000)
	a := lerRede(t, saida.proxima())
	if a.NetRx == nil || a.NetTx == nil {
		t.Fatalf("a segunda amostra veio sem net_rx_bps/net_tx_bps")
	}
	if *a.NetRx < 100 || *a.NetRx > 3000 {
		t.Errorf("net_rx_bps = %v, esperado 3000 bytes divididos por um intervalo entre 1 s e 30 s; com lo, veth, docker, br- ou virbr somados passaria de 30 mil", *a.NetRx)
	}
	if razao := *a.NetRx / *a.NetTx; razao < 2.97 || razao > 3.03 {
		t.Errorf("razão rx/tx = %.3f, esperado 3 (3000 recebidos, 1000 enviados)", razao)
	}
}

func TestStreamMetricsSemNetDevNaoEmiteRede(t *testing.T) {
	saida := iniciarScript(t, "DOCKKEEPER_INTERVAL=1\nDOCKKEEPER_NET_DEV=/caminho/que/nao/existe\n")

	for i := range 2 {
		if a := lerRede(t, saida.proxima()); a.NetRx != nil || a.NetTx != nil {
			t.Errorf("amostra %d sem /proc/net/dev legível trouxe rede (%v, %v)", i+1, a.NetRx, a.NetTx)
		}
	}
}

func enderecosDaLinha(t *testing.T, linha string) []string {
	t.Helper()

	var payload struct {
		Addresses []string `json:"addresses"`
	}
	if err := json.Unmarshal([]byte(linha), &payload); err != nil {
		t.Fatalf("linha do script não é JSON válido: %v (%s)", err, linha)
	}
	return payload.Addresses
}

func TestStreamMetricsDeclaraEnderecosSemVirtuaisNemLoopback(t *testing.T) {
	dir := t.TempDir()
	falso := "#!/bin/sh\ncat <<'SAIDA'\n" +
		"1: lo    inet 127.0.0.1/8 scope host lo\\       valid_lft forever preferred_lft forever\n" +
		"2: eth0    inet 203.0.113.25/24 brd 203.0.113.255 scope global eth0\\       valid_lft forever\n" +
		"3: tailscale0    inet 100.100.0.11/32 scope global tailscale0\\       valid_lft forever\n" +
		"4: docker0    inet 172.17.0.1/16 brd 172.17.255.255 scope global docker0\\       valid_lft forever\n" +
		"5: br-1a2b    inet 172.18.0.1/16 scope global br-1a2b\\       valid_lft forever\n" +
		"6: veth9f2    inet 169.254.1.1/32 scope global veth9f2\\       valid_lft forever\n" +
		"SAIDA\n"
	if err := os.WriteFile(filepath.Join(dir, "ip"), []byte(falso), 0o755); err != nil {
		t.Fatalf("criar ip falso: %v", err)
	}

	linha := iniciarScript(t, "DOCKKEEPER_INTERVAL=60\nPATH="+dir+":$PATH\n").proxima()
	enderecos := enderecosDaLinha(t, linha)

	tem := map[string]bool{}
	for _, a := range enderecos {
		tem[a] = true
	}
	if !tem["203.0.113.25"] || !tem["100.100.0.11"] {
		t.Errorf("addresses=%v, esperado o endereço público e o da overlay", enderecos)
	}
	for _, indesejado := range []string{"127.0.0.1", "172.17.0.1", "172.18.0.1", "169.254.1.1"} {
		if tem[indesejado] {
			t.Errorf("addresses trouxe %q, que é loopback ou interface virtual: %v", indesejado, enderecos)
		}
	}
}

func TestStreamMetricsSemIpNaoQuebraNemEmiteEnderecos(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ip"), []byte("#!/bin/sh\nexit 127\n"), 0o755); err != nil {
		t.Fatalf("criar ip quebrado: %v", err)
	}

	linha := iniciarScript(t, "DOCKKEEPER_INTERVAL=60\nPATH="+dir+":$PATH\n").proxima()
	if enderecos := enderecosDaLinha(t, linha); len(enderecos) != 0 {
		t.Errorf("sem o comando ip a lista deveria sair vazia, veio %v", enderecos)
	}
}
