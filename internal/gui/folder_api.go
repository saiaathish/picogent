package gui

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/saiaathish/picogent/internal/folderpick"
)

func (s *server) folderPickAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	path, err := folderpick.Choose("Select a project folder")
	if errors.Is(err, folderpick.ErrCancelled) {
		w.WriteHeader(204)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"path": path})
}
