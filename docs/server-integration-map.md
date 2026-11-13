# IDE #28 — checked server integration map (2026-11-13)

This is a **read-only inspection of existing server main branches**, not a
claim that the IDE currently authenticates or calls these endpoints. Confirm
Gateway routing and policies before using them from untrusted IDE clients.

## Reuse existing endpoints and owners

| Owner | Existing HTTP surface | Intended IDE use |
| --- | --- | --- |
| Sandbox | `GET /api/v1/sandboxes/:id/tree` and `GET /api/v1/sandboxes/:id/files?path=...` | Inspect permitted existing student code; use revisions from owner responses. |
| Sandbox | `POST /api/v1/sandboxes/:id/files`, `PUT /api/v1/sandboxes/:id/files`, rename/delete variants | Debounced working-file changes, **not** Git pushes. Verify per-student auth, write-lock and fence policy via Gateway. |
| Sandbox | `GET /api/v1/sandboxes/:id/workspace-fence` | Read the current epoch for Practice-backed atomic workspaces. The epoch is **not** itself a content-revision CAS. |
| Sandbox | `POST /api/v1/sandboxes/:id/ws-tickets`, `GET /api/v1/ws/sandboxes/:id` | Optional realtime status; existing WebSocket requires a one-use ticket and allowed origin. Native client policy must be reviewed. |
| Practice Runtime | `GET /api/v2/task-instances`, `GET /api/v2/task-instances/{instanceID}/workspace` | Student-owned task lookup and workspace bindings. |
| Practice Runtime | `POST /api/v3/practice-instances`, `POST /api/v2/task-instances/{instanceID}/submissions` | Existing lesson practice start/official submission. No IDE-specific mastery implementation. |
| Teacher Agent | `GET /api/v1/teacher/me`, `PUT /api/v1/teacher-dialog/ensure` | Existing personal teacher and Dialog-owned context. |
| Course + Learning Planner | Existing material and graph reads | Resolve exact published immutable material before considering generation. Do not make an IDE graph store. |

Evidence: [Sandbox routes](https://github.com/bemulima/ms-go-sandbox/blob/main/internal/transport/http/api/v1/handlers/server.go),
[Practice routes](https://github.com/bemulima/ms-go-practice-runtime/blob/main/internal/transport/http/server.go),
[Teacher contract](https://github.com/bemulima/ms-go-teacher-agent/blob/main/docs/http-contract.md).

## Actual missing contracts (small, separate owner slices)

1. **Trusted IDE session/checkout resolution** at the Gateway/Practice boundary:
   authenticate the student, require active Course access, resolve an available
   lesson, idempotently start or reuse TaskInstance/Sandbox, pin the Course/task
   revision, return an **authorized** `codechuman.checkout.v1` manifest.
   Never accept arbitrary student IDs or provider credentials from the body.
2. **Sandbox read-only Git HTTPS export** for the **starter snapshot only**:
   owner-check TaskInstance/Sandbox, exclude secrets and hidden validation
   files, produce a bounded immutable bare Git export, allow `git-upload-pack`
   but **reject `git-receive-pack` / pushes**. Use a short-lived scoped
   credential via an approved credential helper, not embedded URL userinfo,
   query tokens or command-line arguments. Expiry, cleanup, concurrency and
   rate limits need tests.
3. **Sync adapter using existing file APIs**: watch saved code changes on each
   client; debounce and batch per workspace, ignore `.git/` and `.learning/`,
   limit paths/bytes, handle HTTP auth and server errors. Record the latest
   acknowledged workspace content revision **and** fence epoch, with conflict
   recovery before any official Submit. Never confuse a local edit or Git
   commit with an acknowledged Sandbox revision.
4. **Teacher bounded current-code read**: Practice Runtime's
   `TeacherContext` currently sets
   `Workspace: {Available: false, Files: []}`; it does **not yet return code**.
   Add a permissioned, size-limited, secret-filtered read of an exact
   acknowledged Sandbox snapshot through a trusted internal adapter. The
   Teacher Agent uses that owner-provided context, never arbitrary local paths,
   and stores the conversation in Dialog.
5. **Client UI orchestration**: list available—not locked—Course/Planner nodes,
   show lesson/examples/quiz, start practice, checkout, sync, ask Teacher and
   submit via the **existing** authorized server workflows.

For step (4), [Practice Runtime implementation](https://github.com/bemulima/ms-go-practice-runtime/blob/main/internal/application/usecase/teacher.go)
shows the current `Workspace.Available=false` behavior explicitly. This is
a known integration gap, not a reason to duplicate Teacher or Sandbox.

## Lifecycle invariants to test incrementally

- Cloning is read-only. Editing is local; sync is a separate authorized
  operation; server validation uses an acknowledged snapshot.
- Stale sync, wrong student, changed TaskInstance or fence epoch fail closed.
- Public lesson cache and starter files may be downloaded; private validators,
  internal secrets and a student's API keys may not.
- AI outages or exhausted credits never block deterministic local checks or
  the already authorized ordinary course/practice path.
- Exact Graph/Course reuse and idempotent generation stay with Epic #32;
  funding mode `PLATFORM_SPONSORED` remains the default. Any future BYOK
  generation and publication must follow the explicitly disclosed sharing
  policy; Dialog remains the canonical transcript store.

This map is a contract aid for future tests, **not a substitute for executable
Gherkin, live service tests or end-to-end acceptance**.
