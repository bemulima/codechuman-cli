# Codechuman CLI

Go-based shared local runtime for [Codechuman IDE #28](https://github.com/bemulima/learning-platform-business/issues/28).
Intended for both VS Code and Neovim. **This is a partial client:** the P0
owner-authenticated checkout and immutable Git-read backend are available;
`epic/IDE-28` adds the source path for authenticated public lesson retrieval
and revision-scoped local caching. Native sign-in and full IDE orchestration
remain separate work.

## Implemented

- Versioned `codechuman.checkout.v1` manifest and public content pack.
- HTTPS-only Git clone; no token in the URL, no automatic Git push.
- Immutable lesson/task revision fence and non-destructive checkout reuse.
- Public lesson material checksums and atomic revision-scoped cache under
  `.learning/materials/<lesson_revision>/`; a changed cache is never replaced.
- Local checkout metadata and materials are excluded from Git and remain outside
  the existing Sandbox source-sync allowlist.
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
# After the GUIDED materials route is available, this opens the same pinned
# checkout and retrieves its public lesson pack using the same one-shot token.
# <secure-token-provider> | ./codechuman open-remote \
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
`checkout-remote` creates the pinned starter and identity manifest only.
`open-remote` additionally calls the separate TaskInstance-bound materials
route, validates exact owner identities and every SHA-256, rejects private
paths and quiz answers, then atomically installs the package at
`.learning/materials/<lesson_revision>/`. The server route remains source-only
and default-off until the IDE-28 vertical is integrated and accepted.

The backend P0 flow passed isolated runtime acceptance before being merged;
deployment/activation and a real CLI-command checkout against the full
installed local stack are not yet accepted. Native IDE clients must supply
tokens through secret storage/secure session handoff in a subsequent stage.
Never put a real access token into shell history or a command literal.

**Local safety:** Only run public test code from a trusted publisher. These tests
run as your OS user and can execute arbitrary JavaScript; they are not isolated
by Docker or the platform Sandbox. They are never official graded evidence.

**Not yet implemented:** native login,
WebSocket file sync and revision ACKs,
official Practice Submit, Dialog/Teacher and AI provider execution. A local
`PASS` never grants Mastery.

No open-source license has been selected yet for this public repository.

The [server integration map](docs/server-integration-map.md) identifies existing
Sandbox/Practice/Teacher endpoints to reuse and the smallest missing owner
contracts. It also records the current Teacher code-context gap.

### Process-local private CA (including native macOS)

For an isolated test Gateway, set `CODECHUMAN_CA_FILE` to an operator-provided
PEM CA bundle. CLI adds that bundle to the process's system roots; hostname and
certificate verification remain mandatory. Invalid bundles fail closed. Set
`GIT_SSL_CAINFO` to the same PEM path for native Git. Neither setting modifies
the macOS trust store. Configure these only in your own launcher/environment,
never from a cloned lesson or an untrusted project.

### Task-scoped Teacher chat (IDE28)

`teacher-chat --api-base https://YOUR-GATEWAY --workspace /path/to/checkout
--request-stdin` reads a bounded JSON request from stdin with `token`, optional
`body`, and `idempotency_key` (UUID required when sending). Never place the token
or private message text in argv. The client first reads the actual Student-owned
Runtime TaskInstance, derives its LearningAction from that owner response,
ensures the `practice_task` Teacher conversation, and reads/posts through
canonical Dialog. It does not save credentials or a duplicate transcript.

The result contains `dialog_id`, `task_instance_id`, `messages`, and `pending`.
A pending reply must be refreshed, not blindly resent. Reusing the same message
idempotency key is required after an ambiguous POST. HTTP errors never include
response bodies or the token. Redirects are forbidden and bodies are bounded.

Teacher uses server-owned task/step and assistance policy. Unsynced local code
is not uploaded or treated as official Sandbox state. Real response generation
requires the approved confidential AI Prompt provider; an unavailable provider
must not be replaced with a fabricated answer. Dialog owns all history.
