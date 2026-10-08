package cli

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImageProbeValidatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "image.png")
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 1, 1)))
	os.WriteFile(path, b.Bytes(), 0600)
	parts, err := imageParts(path)
	if err != nil || len(parts) != 2 || !strings.HasPrefix(parts[1].ImageURL.URL, "data:image/png;base64,") {
		t.Fatal("valid image rejected", err)
	}
	os.WriteFile(path, b.Bytes()[:33], 0600)
	if _, err = imageParts(path); err == nil {
		t.Fatal("header-valid truncated PNG accepted")
	}
	os.WriteFile(path, []byte("not an image"), 0600)
	if _, err = imageParts(path); err == nil {
		t.Fatal("fake image accepted")
	}
	os.WriteFile(path, make([]byte, (4<<20)+1), 0600)
	if _, err = imageParts(path); err == nil {
		t.Fatal("oversize image accepted")
	}
}

func TestVisionProbeRejectsGIFBeforeUpload(t *testing.T) {
	var b bytes.Buffer
	frame := image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{color.Black, color.White})
	if err := gif.EncodeAll(&b, &gif.GIF{Image: []*image.Paletted{frame, frame}, Delay: []int{1, 1}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "animated.gif")
	os.WriteFile(path, b.Bytes(), 0600)
	if _, err := imageParts(path); err == nil {
		t.Fatal("unbounded animated GIF passed PoC validation")
	}
}

func TestMissingCredentialsNeverRunProbes(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "")
	t.Setenv("OLLAMA_API_KEY", "")
	for _, command := range []string{"deepseek", "stream", "search"} {
		var out bytes.Buffer
		if err := Run([]string{"poc", command}, &out); err == nil {
			t.Fatal("missing credentials accepted", command)
		}
	}
}
