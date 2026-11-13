# Codechuman CLI

Go-based shared local runtime for [Codechuman IDE #28](https://github.com/bemulima/learning-platform-business/issues/28).
Used by both VS Code and Neovim. **This is a partial source implementation,
not a finished connected Codechuman client.**

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
./codechuman list
./codechuman validate-js --workspace ~/.codechuman/js-beginner/01 --file app.js
./codechuman validate-public-js --workspace ~/.codechuman/js-beginner/01
```

The root defaults to `~/.codechuman` and can be overridden with
`CODECHUMAN_WORKSPACE_ROOT` or `--root`. Each lesson has a standalone `.git`.

## Checkout JSON contract (draft)

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
  "clone_url": "https://YOUR-CODECHUMAN-HOST/api/v1/sandbox/git/exports/exp-123.git",
  "materials": [
    { "path": "lesson.md", "body": "# JavaScript Variables\n" },
    { "path": "examples/basic.md", "body": "const x = 1;\n" },
    { "path": "tests/public/answer.test.js", "body": "// Trusted public Node test\n" }
  ]
}
```

Only a server-authorized and owner-bound checkout spec may be used in the final
connected flow. In this slice the CLI loads a local JSON file for deterministic
testing; authenticated fetching and credential helpers are a **future integration**.
The example URL is illustrative, not an already deployed endpoint.

**Local safety:** Only run public test code from a trusted publisher. These tests
run as your OS user and can execute arbitrary JavaScript; they are not isolated
by Docker or the platform Sandbox. They are never official graded evidence.

**Not yet implemented:** live Course/Student fetching, authenticated Git HTTP
export in Sandbox, WebSocket file sync and revision ACKs, official Practice
Submit, Dialog/Teacher and AI provider execution. A local `PASS` never grants
Mastery.

No open-source license has been selected yet for this public repository.

The [server integration map](docs/server-integration-map.md) identifies existing
Sandbox/Practice/Teacher endpoints to reuse and the smallest missing owner
contracts. It also records the current Teacher code-context gap.
