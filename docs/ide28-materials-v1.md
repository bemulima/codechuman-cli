# IDE28-08A — Pinned learner-visible materials (proposed contract)

Status: **CLI consumer implemented on `epic/IDE-28`; server integration and acceptance pending**. Parent: [IDE-28 / business #52](https://github.com/bemulima/learning-platform-business/issues/52). The route stays source-only and default-off until the complete server vertical is reviewed; no production flag or login flow is enabled.

## Why a separate contract

The current `POST /api/v1/ide/checkouts` returns `codechuman.checkout.v1`: owner-pinned identity and a read-only Git starter URL. It deliberately returns **no** Course content. The public Course `/api/v1/lesson-materials/get/{lessonID}` uses current lesson state, not the exact immutable Student admission. The internal `lesson-revision.v3/v4` payload may contain private quiz grading keys and test/solution data; it must **never** be forwarded unchanged to students.

A future Practice endpoint, behind authenticated Gateway ingress, is proposed:

`GET /api/v1/ide/checkouts/{canonicalTaskInstanceUUID}/materials`

- Only the TaskInstance ID appears in the URL. No caller-supplied Student/Course/Lesson/revision identifiers.
- One original Student Bearer, trusted Gateway identity and a distinct default-OFF route capability. No cookie, redirect, direct admin/internal alias, query string, token in URL, or unauthenticated discovery.
- Practice must re-read its persisted owner-scoped TaskInstance, the **fresh** Student IDE eligibility proof (pinned `lesson_revision_id` and Course/structure receipt), and the Course owner placement proof before requesting any material. Correlate Student, TaskInstance, Course, Lesson, revision, TaskRevision and immutable publication identities exactly. Foreign/stale/unselected/disabled must fail closed.
- Course owns the immutable revision and creates an **explicit allowlist projection** of title, theory/description and approved examples, never a generic marshalling of the private revision DTO.
- Practice Runtime owns the authenticated TaskInstance and composes the IDE response. **Practice Catalog** (`ms-go-pet-project-orchestrator`) owns the immutable imported task publication and its human-approved student-safe description; see [Catalog Draft #48](https://github.com/bemulima/ms-go-pet-project-orchestrator/pull/48). The private imported revision is intentionally unavailable through legacy public/current Catalog routes: resolve only the exact approved publication receipt after checking Practice TaskInstance and ACTIVE Course admission pins. An ordinary/archive task requires its own separately reviewed source projection; no fallback to a mutable Catalog DTO.
- Only separately authored, explicitly learner-public tests may be downloadable. No private validator cases, solution code, expected outputs, execution mappings, internal prompts or mutable Sandbox workspace. Until such a test policy exists, omit `tests/public/...`.
- Quiz is a **view-only** projection of prompt and choices. No `correct_choice_key`, `is_correct`, `correct_order`, scoring/rubric metadata or hidden variants; server scoring remains authoritative. If no safe quiz projection is available, omit `quiz.json` until its owner contract is ready.
- Preserve the current checkout API, Git export and feature flags. Never produce a fresh AI-generated lesson or alter Epic #32.

## Proposed immutable JSON response

```json
{
  "schema": "codechuman.ide-materials.v1",
  "task_instance_id": "22222222-2222-4222-8222-222222222222",
  "course_id": "88888888-8888-4888-8888-888888888888",
  "lesson_id": "99999999-9999-4999-8999-999999999999",
  "lesson_revision": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
  "files": [
    {
      "path": "lesson.md",
      "body": "# Immutable lesson content\n",
      "sha256": "sha256:<64 lowercase hexadecimal characters>"
    }
  ]
}
```

The SHA-256 placeholder above is descriptive, **not** a valid digest; the server must calculate it over exact UTF-8 `body` bytes. Other allowed paths are `task.md`, `quiz.json`, `examples/...`, and `tests/public/...`. Clients require `lesson.md`. Paths are case-insensitively unique; total material bytes <=4 MiB, <=512 KiB per file, <=100 files, response <=6 MiB, no symlink/traversal/hidden components. Content-Type is `application/json`, Cache-Control includes `no-store`. Do not put bearer, secret or private asset in JSON, logs or workspace.

`quiz.json` schema, when approved:

```json
{
  "schema": "codechuman.quiz-view.v1",
  "title": "Practice quiz",
  "questions": [
    {"key":"q1","prompt":"Question","choices":[{"key":"a","text":"A"},{"key":"b","text":"B"}]}
  ]
}
```

Only this answer-free view is accepted by the CLI parser. Quiz grading still happens on the server.

## Consumer readiness and cache rule

`codechuman open-remote` uses HTTPS with redirect prevention and a bearer only in request headers. It compares all bundle identities with the separately authorized Git checkout Spec, verifies each checksum and the existing strict public material path/size limits, then atomically caches files under `.learning/materials/<lesson_revision>/`. The cache is bound to `.learning/manifest.json`; it verifies existing cache contents and refuses to replace modified data or symlinks. It does not change starter source files.

A bounded native HTTPS fixture exercises `open-remote`, Git clone, material SHA validation, atomic cache reuse and preservation of student source edits. This fixture is not live Auth/RBAC or owner acceptance. A real authorized Gateway request remains part of the final vertical smoke. VS Code and Neovim read only the revision-scoped local package.

## Proposed targeted acceptance

- Authorized Student, exact TaskInstance/revision/file digests, HTTPS no-store: PASS.
- Guest, foreign/disabled/stale/mismatched revision or Course proof: DENY at Gateway/Practice owners.
- Unknown JSON fields, corrupt checksum, private path, traversal, duplicate case-fold path, oversized pack, secret grading keys and HTTP redirect: DENY at consumer.
- No token persisted, no AI generation, no Git push, no local Mastery.
- Only one isolated owner-backed integration smoke after server slices IDE28-08B/C, without replaying IDE28-07 or starting full Playwright.

**Current verification:** Go 1.24.4; `go test ./internal/checkout -run '^TestCacheRemoteMaterials' -count=1` PASS; `go test ./cmd/codechuman -run '^TestOpenRemoteCommandFetchesAndCachesPinnedPublicLesson$' -count=1 -v` PASS against an isolated local HTTPS/Git fixture. This does not exercise live Auth/RBAC, Gateway, Practice, Course, a foreign-student denial, or the final cross-service smoke.
