package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/projects"
	"github.com/saiaathish/picogent/internal/session"
)

func TestProjectsAPIListsChatsUnderTheirProjects(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	firstPath, secondPath := t.TempDir(), t.TempDir()
	first, err := projects.Add("first", firstPath)
	if err != nil {
		t.Fatal(err)
	}
	second, err := projects.Add("second", secondPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.New(firstPath).Save(); err != nil {
		t.Fatal(err)
	}
	if err := session.New(secondPath).Save(); err != nil {
		t.Fatal(err)
	}
	s := &server{cfg: config.Config{Workspace: secondPath}}
	res := httptest.NewRecorder()
	s.projectsAPI(res, httptest.NewRequest(http.MethodGet, "/api/projects", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var body struct {
		CurrentID string                    `json:"current_id"`
		Chats     map[string][]session.Meta `json:"chats"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.CurrentID != second.ID || len(body.Chats[first.ID]) != 1 || len(body.Chats[second.ID]) != 1 {
		t.Fatalf("current=%q chats=%v", body.CurrentID, body.Chats)
	}
}
