package validation

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func publicFixture(t *testing.T, source, assertion string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".learning", "tests", "public")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "app.js"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	if assertion != "" {
		if err := os.WriteFile(filepath.Join(dir, "solution.test.js"), []byte(assertion), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestPublicJavaScriptTestsPassAndFailWithoutAI(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("Node.js is not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	assertion := "const {test}=require('node:test'); const assert=require('node:assert/strict'); const app=require('../../../app.js'); test('answer',()=>assert.equal(app.answer(), 42));\n"
	good := publicFixture(t, "module.exports.answer = () => 42;\n", assertion)
	if result := PublicJavaScriptTests(ctx, good); result.Status != "PASS" {
		t.Fatalf("expected PASS, got %+v", result)
	}
	bad := publicFixture(t, "module.exports.answer = () => 41;\n", assertion)
	if result := PublicJavaScriptTests(ctx, bad); result.Status != "FAIL" {
		t.Fatalf("expected FAIL, got %+v", result)
	}
}

func TestPublicJavaScriptTestsReportMissingAndUnsafeTests(t *testing.T) {
	root := publicFixture(t, "module.exports.answer = () => 42;\n", "")
	if result := PublicJavaScriptTests(context.Background(), root); result.Status != "NO_TESTS" {
		t.Fatalf("expected NO_TESTS, got %+v", result)
	}
	foreign := t.TempDir()
	dir := filepath.Join(root, ".learning", "tests", "public")
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, dir); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if result := PublicJavaScriptTests(context.Background(), root); result.Status != "FAIL" {
		t.Fatalf("expected refusal of symlink, got %+v", result)
	}
}
