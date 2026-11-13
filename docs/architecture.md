# IDE #28 — owner boundaries and future-proof execution

This document describes **planned contracts**; only the code under `internal/`
and `cmd/` is implemented in this slice.

## Existing authoritative owners

- **ms-go-learning-planner**: Knowledge Graph/Curriculum and existing compatible
  material resolution. Do not duplicate Graph authoring in any IDE client.
- **ms-go-course**: published immutable lessons, examples and public quizzes.
- **ms-go-practice-runtime / ms-go-student**: TaskInstance, Submission, evidence
  and Mastery.
- **ms-go-sandbox**: live workspace, file revisions, authoritative snapshots.
- **ms-go-validation-orchestrator**: deterministic official grading.
- **ms-go-teacher-agent / ms-go-dialog / ms-go-ai-prompt**: teacher decisions,
  canonical conversation history and controlled AI execution.

## Planned checkout lifecycle

1. Auth/Student authorizes one exact available lesson and TaskInstance.
2. Sandbox prepares a **read-only**, secret-free Git HTTPS export from the task
   starter files, not from the live grading workspace's `.git` directory.
3. Course exposes permitted materials and an immutable content revision. The
   server issues a version-pinned `checkout.v1` manifest after owner checks.
4. The CLI clones task code into a configurable course/lesson directory and
   stores public materials in `.learning/`. Hidden tests must never be exported.
5. Subsequent file edits sync via Sandbox HTTP/WS with revision ACKs, not Git
   commits. Metadata and lesson files are excluded from the sync allowlist.
6. Teacher may read only the acknowledged, student-owned bounded code snapshot.
7. Official Submit and Mastery remain owned by Practice Runtime/Student, not the
   IDE's local validator.

## AI policy (future phases)

Every AI request must have **independent** trusted attributes:

- execution purpose (`TEACHER_CHAT`, `LESSON_GENERATION`, ...),
- payer (`PLATFORM_SPONSORED` **default**, `STUDENT_CREDITS`, `STUDENT_BYOK`,
  `STUDENT_DELEGATED` only when officially permitted),
- result scope (`PLATFORM_SHARED` or `STUDENT_PRIVATE`),
- verified student/session/learning-path/task refs, provider connection ref,
- immutable prompt/policy version and idempotency key.

Funding never determines visibility. Student-funded generated lessons/tasks may
enter the normal Planner/Course/TZ publish/reuse path after transparent notice,
consent, deterministic validation and no personal information leakage. If a
compatible material already exists, no model call is made. Dialog messages
always persist in `ms-go-dialog`, even for future local provider adapters.

AI availability and AI credits must never be prerequisites for ordinary local
checks, permitted Course reads, Sandbox sync or deterministic official Submit.

Use additive versioned contracts: do not rewrite current `execute-confidential`
or Epic #32's immutable Graph/Course status and review machinery.
