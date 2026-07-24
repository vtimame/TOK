package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCLIUIOpenAPIWritesSpecToStdoutOrFile(t *testing.T) {
	ctx := context.Background()

	var out bytes.Buffer
	cli := NewCLI(&out, &bytes.Buffer{}, VersionInfo{})
	if err := cli.Run(ctx, []string{"ui", "openapi"}); err != nil {
		t.Fatalf("ui openapi returned error: %v", err)
	}
	spec := decodeOpenAPISpec(t, out.Bytes())
	if spec.OpenAPI == "" || spec.Paths["/api/projects/{project}/tasks"]["post"].OperationID != "createTask" {
		t.Fatalf("unexpected stdout OpenAPI spec: %+v", spec)
	}

	out.Reset()
	target := filepath.Join(t.TempDir(), "generated", "openapi.json")
	if err := cli.Run(ctx, []string{"ui", "openapi", "--out", target}); err != nil {
		t.Fatalf("ui openapi --out returned error: %v", err)
	}
	if out.String() != "wrote OpenAPI spec: "+target+"\n" {
		t.Fatalf("unexpected --out message: %q", out.String())
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read generated OpenAPI spec: %v", err)
	}
	spec = decodeOpenAPISpec(t, data)
	if spec.Paths["/api/projects"]["get"].OperationID != "listProjects" {
		t.Fatalf("unexpected file OpenAPI spec: %+v", spec)
	}
}

func decodeOpenAPISpec(t *testing.T, data []byte) uiOpenAPITestSpec {
	t.Helper()
	var spec uiOpenAPITestSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("decode OpenAPI spec: %v\n%s", err, string(data))
	}
	return spec
}

type uiOpenAPITestSpec struct {
	OpenAPI string                                       `json:"openapi"`
	Paths   map[string]map[string]uiOpenAPITestOperation `json:"paths"`
}

type uiOpenAPITestOperation struct {
	OperationID string `json:"operationId"`
}
