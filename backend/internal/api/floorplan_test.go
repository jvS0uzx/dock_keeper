package api

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// pngBytes gera um PNG válido do tamanho pedido, para exercitar a validação
// sem depender de arquivo de fixture.
func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

func uploadPart(t *testing.T, data []byte) multipart.File {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("image", "planta.png")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/floorplans", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if err := req.ParseMultipartForm(maxPlanUploadBytes); err != nil {
		t.Fatalf("ParseMultipartForm: %v", err)
	}
	file, _, err := req.FormFile("image")
	if err != nil {
		t.Fatalf("FormFile: %v", err)
	}
	t.Cleanup(func() { file.Close() })
	return file
}

func TestStorePlanImageAceitaPNG(t *testing.T) {
	t.Setenv("FLOORPLAN_DIR", t.TempDir())

	stored, err := storePlanImage(uploadPart(t, pngBytes(t, 800, 600)))
	if err != nil {
		t.Fatalf("storePlanImage: %v", err)
	}
	if stored.Width != 800 || stored.Height != 600 {
		t.Errorf("dimensões = %dx%d, esperado 800x600", stored.Width, stored.Height)
	}
	if stored.ContentType != "image/png" {
		t.Errorf("content type = %q", stored.ContentType)
	}
	if _, err := os.Stat(stored.Path); err != nil {
		t.Errorf("arquivo não foi gravado: %v", err)
	}
}

// O tipo sai do conteúdo, nunca do Content-Type declarado: um HTML servido pelo
// endpoint da imagem viraria XSS no painel.
func TestStorePlanImageRecusaConteudoQueNaoEImagem(t *testing.T) {
	t.Setenv("FLOORPLAN_DIR", t.TempDir())

	_, err := storePlanImage(uploadPart(t, []byte("<html><script>alert(1)</script></html>")))
	if err == nil {
		t.Fatal("conteúdo HTML foi aceito como planta")
	}
}

func TestStorePlanImageRecusaImagemMinuscula(t *testing.T) {
	t.Setenv("FLOORPLAN_DIR", t.TempDir())

	if _, err := storePlanImage(uploadPart(t, pngBytes(t, 10, 10))); err == nil {
		t.Fatal("imagem 10x10 foi aceita")
	}
}

func TestStorePlanImageRecusaArquivoVazio(t *testing.T) {
	t.Setenv("FLOORPLAN_DIR", t.TempDir())

	if _, err := storePlanImage(uploadPart(t, nil)); err == nil {
		t.Fatal("arquivo vazio foi aceito")
	}
}

// O nome em disco é aleatório, então dois envios do mesmo arquivo não colidem
// nem permitem adivinhar o caminho de outra planta.
func TestStorePlanImageGeraNomeAleatorio(t *testing.T) {
	t.Setenv("FLOORPLAN_DIR", t.TempDir())

	data := pngBytes(t, 400, 300)
	primeira, err := storePlanImage(uploadPart(t, data))
	if err != nil {
		t.Fatalf("primeiro envio: %v", err)
	}
	segunda, err := storePlanImage(uploadPart(t, data))
	if err != nil {
		t.Fatalf("segundo envio: %v", err)
	}
	if primeira.Path == segunda.Path {
		t.Fatalf("mesmo caminho nos dois envios: %s", primeira.Path)
	}
}

func TestPlanIDFromPath(t *testing.T) {
	cases := []struct {
		path, suffix string
		want         uint
		ok           bool
	}{
		{"/api/floorplans/7", "", 7, true},
		{"/api/floorplans/7/image", "/image", 7, true},
		{"/api/floorplans/12/pins", "/pins", 12, true},
		{"/api/floorplans/abc", "", 0, false},
		{"/api/floorplans/0", "", 0, false},
		{"/api/floorplans/", "", 0, false},
	}

	for _, c := range cases {
		rec := httptest.NewRecorder()
		got, ok := planIDFromPath(rec, httptest.NewRequest(http.MethodGet, c.path, nil), c.suffix)
		if ok != c.ok || got != c.want {
			t.Errorf("%s: (%d,%v), esperado (%d,%v)", c.path, got, ok, c.want, c.ok)
		}
	}
}

// Coordenada fora da imagem viraria marcador invisível.
func TestClampPercent(t *testing.T) {
	for input, want := range map[float64]float64{-5: 0, 0: 0, 42.5: 42.5, 100: 100, 180: 100} {
		if got := clampPercent(input); got != want {
			t.Errorf("clampPercent(%v) = %v, esperado %v", input, got, want)
		}
	}
}

func TestFloorPlanRouterRejeitaMetodo(t *testing.T) {
	cases := map[string]struct{ path, method string }{
		"imagem com POST":  {"/api/floorplans/1/image", http.MethodPost},
		"pins com GET":     {"/api/floorplans/1/pins", http.MethodGet},
		"planta com PATCH": {"/api/floorplans/1", http.MethodPatch},
	}

	for name, c := range cases {
		rec := httptest.NewRecorder()
		floorPlanRouter(rec, httptest.NewRequest(c.method, c.path, nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: status = %d, esperado %d", name, rec.Code, http.StatusMethodNotAllowed)
		}
		if rec.Header().Get("Allow") == "" {
			t.Errorf("%s: resposta sem cabeçalho Allow", name)
		}
	}
}
