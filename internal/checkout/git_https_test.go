package checkout

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAuthenticatedGitCloneUsesScopedEnvironmentNotURLOrArguments(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fake Git helper test")
	}
	root := t.TempDir()
	bin := filepath.Join(root, "fake-git")
	// A fake Git binary checks its environment without printing the bearer.
	contents := `#!/bin/sh
if [ "$GIT_CONFIG_COUNT" != "3" ]; then exit 4; fi
if [ "$GIT_CONFIG_VALUE_0" != "false" ]; then exit 5; fi
if [ "$GIT_CONFIG_VALUE_1" != "Authorization: Bearer test.jwt.token" ]; then exit 6; fi
if [ "$GIT_CONFIG_VALUE_2" != "" ]; then exit 7; fi
case "$*" in *test.jwt.token*) exit 8 ;; esac
case "$*" in *--depth*) exit 10 ;; esac
mkdir -p "$5/.git/info" || exit 9
printf 'module.exports.answer=42;\n' > "$5/app.js"
`
	if err := os.WriteFile(bin, []byte(contents), 0o700); err != nil {
		t.Fatal(err)
	}
	spec := specimen()
	spec.CloneURL = "https://api.codechuman.example/api/v1/ide/git/exports/one.git"
	result, err := Checkout(context.Background(), filepath.Join(root, "workspace"), spec,
		GitCloner{Binary: bin, BearerToken: "test.jwt.token"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(result.Directory, "app.js")); err != nil {
		t.Fatalf("Git helper did not deliver starter source: %v", err)
	}
}
