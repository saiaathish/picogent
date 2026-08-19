package gui_test

import (
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/gui"
)

func TestEmbeddedIndex(t *testing.T) {
	b, err := gui.ReadWeb("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "picogent") {
		t.Fatal("index missing title")
	}
}
