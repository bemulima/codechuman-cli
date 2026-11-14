# IDE #28 — server integration map (updated 2026-11-14)

The owner-authenticated Practice Git starter P0 path and descriptor endpoint
have been merged to main, but remain **feature-flagged off until a controlled
activation**. The remaining sync, Teacher and full Course UI work below is not
implied by that acceptance.

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

## Owner contracts and remaining integration work

1. **Merged P0 descriptor for an already provisioned TaskInstance**:
   Gateway authenticates the Student and Practice verifies persisted TaskInstance
   ownership, Student's current path/assignment and Course immutable placement.
   It does **not** create or start a new instance; callers must use the existing
   authorized Start flow first. Never accept arbitrary student IDs or provider
   credentials from the body. The exact owner contract is:

   - `POST /api/v1/ide/checkouts` (student JWT, no client-supplied student ID).
   - Request: `{"task_instance_id":"<UUID>"}` with no other fields.
   - Response: HTTP 200, `codechuman.checkout.v1` with exact Course/Lesson,
     Sandbox/TaskInstance identity and **relative**
     `git_url: "/api/v1/ide/git/exports/<TaskInstance UUID>.git"`.
   - Lesson content is not included. A foreign, stale or unready TaskInstance
     fails closed; Course material and Task Start remain separate owner flows.
2. **Merged Practice-owned read-only dumb Git HTTPS export** of the immutable
   starter snapshot (including pinned Catalog/FileStorage ZIP). Gateway repeats
   Student authentication for each resource; Practice checks owner TaskInstance;
   hidden validation assets and mutable Sandbox files are never exported.
   Git push, receive-pack and shallow transport remain unsupported. The CLI
   scopes its ephemeral bearer to the exact HTTPS Git URL via environment, not
   URL userinfo, query tokens, argv, Git config or credential helpers.
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
