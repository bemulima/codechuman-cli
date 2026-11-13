package validation

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestJavaScriptSyntaxWorksWithoutAIOrSandbox(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("Node runtime unavailable on this test host")
	}
	root := t.TempDir()
	file := filepath.Join(root, "app.js")
	if err := os.WriteFile(file, []byte("function answer() { return 42; }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := JavaScriptSyntax(context.Background(), root, "app.js")
	if result.Status != "PASS" {
		t.Fatalf("valid JS should pass locally: %+v", result)
	}
	if err := os.WriteFile(file, []byte("function answer( {\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result = JavaScriptSyntax(context.Background(), root, "app.js")
	if result.Status != "FAIL" || result.Details == "" {
		t.Fatalf("invalid JS should report a deterministic diagnostic: %+v", result)
	}
}

func TestJavaScriptSyntaxDoesNotReadPrivateOrOutsideFiles(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"../outside.js", "/tmp/outside.js", ".git/config", ".learning/tests/private/secret.js", "dir\\app.js"} {
		result := JavaScriptSyntax(context.Background(), root, name)
		if result.Status != "FAIL" {
			t.Fatalf("expected invalid target %q to fail: %+v", name, result)
		}
	}
}
