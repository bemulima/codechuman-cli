# IDE28-08A — Pinned learner-visible materials (proposed contract)

Status: **consumer source proposal, NOT integrated**. Parent: [IDE28-08 / business #52](https://github.com/bemulima/learning-platform-business/issues/52). No server route, feature flag, deployment, login flow or material cache write is introduced by this change.

## Why a separate contract

The current `POST /api/v1/ide/checkouts` returns `codechuman.checkout.v1`: owner-pinned identity and a read-only Git starter URL. It deliberately returns **no** Course content. The public Course `/api/v1/lesson-materials/get/{lessonID}` uses current lesson state, not the exact immutable Student admission. The internal `lesson-revision.v3/v4` payload may contain private quiz grading keys and test/solution data; it must **never** be forwarded unchanged to students.

A future Practice endpoint, behind authenticated Gateway ingress, is proposed:

`GET /api/v1/ide/checkouts/{canonicalTaskInstanceUUID}/materials`

- Only the TaskInstance ID appears in the URL. No caller-supplied Student/Course/Lesson/revision identifiers.
- One original Student Bearer, trusted Gateway identity and a distinct default-OFF route capability. No cookie, redirect, direct admin/internal alias, query string, token in URL, or unauthenticated discovery.
- Practice must re-read its persisted owner-scoped TaskInstance, the **fresh** Student IDE eligibility proof (pinned `lesson_revision_id` and Course/structure receipt), and the Course owner placement proof before requesting any material. Correlate Student, TaskInstance, Course, Lesson, revision, TaskRevision and immutable publication identities exactly. Foreign/stale/unselected/disabled must fail closed.
- Course owns the immutable revision and creates an **explicit allowlist projection** of title, theory/description and approved examples, never a generic marshalling of the private revision DTO.
- Practice owns the bound public task statement and only tests explicitly approved for student download. No private validator cases, solution code, expected outputs, internal prompts or mutable Sandbox workspace.
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

The unmounted `checkout.ResolveRemoteMaterials` uses HTTPS with redirect prevention and a bearer only in the HTTP header. It compares all bundle identities with the separately authorized Git checkout Spec, verifies each checksum and reuses the existing strict public material path/size limits; it returns validated in-memory files **without writing them**.

A later IDE28-08D change must add a crash-safe, revision-fenced cache transaction under `.learning/` with the existing checkout manifest and `.git/info/exclude` rules. An existing checkout and student source file must never be overwritten by a material refresh. VS Code and Neovim already have local read-only material readers. No CLI command currently calls this parser: no partial data is advertised as fetched.

## Proposed targeted acceptance

- Authorized Student, exact TaskInstance/revision/file digests, HTTPS no-store: PASS.
- Guest, foreign/disabled/stale/mismatched revision or Course proof: DENY at Gateway/Practice owners.
- Unknown JSON fields, corrupt checksum, private path, traversal, duplicate case-fold path, oversized pack, secret grading keys and HTTP redirect: DENY at consumer.
- No token persisted, no AI generation, no Git push, no local Mastery.
- Only one isolated owner-backed integration smoke after server slices IDE28-08B/C, without replaying IDE28-07 or starting full Playwright.

**Current verification:** Go compiler is not available in this workspace (the `go` executable here is a board game). Go tests/build/gofmt are **NOT_RUN** until a suitable Go toolchain is provided. This is not a GREEN runtime release.
