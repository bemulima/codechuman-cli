# Codechuman CLI

Go-based shared local runtime for [Codechuman IDE #28](https://github.com/bemulima/learning-platform-business/issues/28).
Intended for both VS Code and Neovim. **This is a partial client:** the P0
owner-authenticated checkout and immutable Git-read backend are merged, but
client sign-in, lesson material retrieval, and full IDE orchestration remain
separate work.

## Implemented

- Versioned `codechuman.checkout.v1` manifest and public content pack.
- HTTPS-only Git clone; no token in the URL, no automatic Git push.
- Immutable lesson/task revision fence and non-destructive checkout reuse.
- Locally cached `lesson.md`, `task.md`, examples and public tests under
  `.learning/`, excluded from Git status and reserved from future Sandbox sync.
- JS syntax check with the installed Node.js runtime, independent of AI credits.
- Explicit opt-in for public JavaScript `*.test.js` checks, also AI-independent;
  no tests returns `NO_TESTS` rather than a misleading success.
- A JSON CLI consumed by both editors.

```sh
go test ./...
go build -o codechuman ./cmd/codechuman

./codechuman doctor
./codechuman checkout --spec /path/to/authorized-checkout.json
# Use a short-lived Student access token supplied by the authenticated IDE
# session over stdin, never as a command-line argument or embedded Git URL.
# <secure-token-provider> | ./codechuman checkout-remote \
#   --api-base https://YOUR-CODECHUMAN-HOST \
#   --task-instance 00000000-0000-4000-8000-000000000028 --token-stdin
./codechuman list
./codechuman validate-js --workspace ~/.codechuman/js-beginner/01 --file app.js
./codechuman validate-public-js --workspace ~/.codechuman/js-beginner/01
```

The root defaults to `~/.codechuman` and can be overridden with
`CODECHUMAN_WORKSPACE_ROOT` or `--root`. Each lesson has a standalone `.git`.

## Local checkout JSON contract

```json
{
  "schema": "codechuman.checkout.v1",
  "course_id": "course-123",
  "course_slug": "js-beginner",
  "lesson_id": "lesson-456",
  "lesson_number": 1,
  "lesson_revision": "rev-789",
  "task_instance_id": "task-123",
  "sandbox_id": "sandbox-456",
  "clone_url": "https://YOUR-CODECHUMAN-HOST/api/v1/ide/git/exports/00000000-0000-4000-8000-000000000028.git",
  "materials": [
    { "path": "lesson.md", "body": "# JavaScript Variables\n" },
    { "path": "examples/basic.md", "body": "const x = 1;\n" },
    { "path": "tests/public/answer.test.js", "body": "// Trusted public Node test\n" }
  ]
}
```

`checkout --spec` is retained for local deterministic testing with a previously
authorized manifest and optional public lesson materials. It must not be used
as a way to bypass server authorization for private practice. The example URL
is illustrative, not an activated deployment.

### Authenticated remote checkout (IDE28 P0)

`checkout-remote` calls the now-implemented, default-off
`POST /api/v1/ide/checkouts` Gateway endpoint using an **ephemeral Student
access token from a pipe**. The request contains only
`{"task_instance_id":"<UUID>"}`. Practice returns a bounded
`codechuman.checkout.v1` descriptor with an exact **relative**
`git_url: "/api/v1/ide/git/exports/<TaskInstance UUID>.git"`.

CLI validates the bounded owner ID formats, exact TaskInstance Git path,
schema and same-origin HTTPS transport (the server verifies actual ownership),
then runs native Git with an ephemeral
path-scoped Authorization header. The token never appears in Git argv, clone
URL, saved manifest or `.git/config`. HTTP, redirects, stale/foreign TaskInstance
identities and unexpected descriptor fields fail closed. The Git transport is
read-only and is owned by Practice Runtime, not Sandbox Git push.

The remote descriptor contains **no lesson.md, examples, quiz or public tests**.
Those materials require a separate authorized Course read, so
`checkout-remote` currently creates the pinned starter and identity manifest
only. Local validation of already available public source still works offline.

The backend P0 flow passed isolated runtime acceptance before being merged;
deployment/activation and a real CLI-command checkout against the full
installed local stack are not yet accepted. Native IDE clients must supply
tokens through secret storage/secure session handoff in a subsequent stage.
Never put a real access token into shell history or a command literal.

**Local safety:** Only run public test code from a trusted publisher. These tests
run as your OS user and can execute arbitrary JavaScript; they are not isolated
by Docker or the platform Sandbox. They are never official graded evidence.

**Not yet implemented:** live lesson/material fetching and native login,
WebSocket file sync and revision ACKs,
official Practice Submit, Dialog/Teacher and AI provider execution. A local
`PASS` never grants Mastery.

No open-source license has been selected yet for this public repository.

The [server integration map](docs/server-integration-map.md) identifies existing
Sandbox/Practice/Teacher endpoints to reuse and the smallest missing owner
contracts. It also records the current Teacher code-context gap.
