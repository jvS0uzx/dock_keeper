package discovery

import (
	"os"
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/bancoteste"
)

func TestMain(m *testing.M) {
	os.Exit(bancoteste.Main(m))
}
