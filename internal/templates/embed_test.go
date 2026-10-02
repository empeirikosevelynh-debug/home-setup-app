package templates_test

import (
	"encoding/json"
	"golden-gate-setup/internal/templates"
	"testing"
)

func TestWorkspaceTemplatesEmbedded(t *testing.T) {
	for _, lang := range []string{"go", "crystal", "nim"} {
		for _, file := range []string{"settings.json", "tasks.json"} {
			data, err := templates.Load("workspaces/" + lang + "/zed/" + file)
			if err != nil {
				t.Fatal(err)
			}
			var value any
			if json.Unmarshal(data, &value) != nil {
				t.Fatal("workspace is not valid JSON")
			}
		}
	}
}
func TestTemplateTraversalRejected(t *testing.T) {
	if _, err := templates.Load("../go.mod"); err == nil {
		t.Fatal("template lookup escaped assets")
	}
}
