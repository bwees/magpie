package app

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"magpie/internal/platform/config"
	"magpie/internal/platform/httpserver"
)

const committedSpecPath = "../../" + httpserver.SpecFile

// TestMessageSchemasAreRegenerated fails when an event struct changed without `mise run openapi`,
// which would leave the web client's live-event types stale.
func TestMessageSchemasAreRegenerated(t *testing.T) {
	server := httpserver.Build(config.Env{}, httpserver.ModeServe, nil)
	httpserver.RegisterSchemas(server, messageSchemas()...)
	generated := normalise(t, server.OpenAPI.Description().Components.Schemas)

	raw, err := os.ReadFile(committedSpecPath)
	if err != nil {
		t.Fatal(err)
	}
	var committed struct {
		Components struct {
			Schemas map[string]any `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &committed); err != nil {
		t.Fatal(err)
	}

	for name, schema := range generated {
		if !reflect.DeepEqual(schema, committed.Components.Schemas[name]) {
			t.Errorf("schema %s differs from %s; run `mise run openapi`", name, httpserver.SpecFile)
		}
	}
}

func normalise(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
