@ide28 @contract-only @not-implemented
Feature: Codechuman IDE learning without a browser editor
  The IDE is a second UI for the existing student-owned Course, Practice,
  Sandbox and Teacher workflows. This file is a requirements contract until
  real-owner step definitions are added to learning-platform-verification.

  @p0 @git
  Scenario: The student clones only an authorized task over HTTPS
    Given a student has access to an active published JavaScript lesson
    And a matching TaskInstance and Sandbox belong to that student
    When the IDE requests a read-only HTTPS Git export
    Then git clone obtains the published starter files
    And a push to the Git export is rejected
    And hidden validators and secrets are absent from the checkout

  @p0 @cache
  Scenario: Permitted lesson content is stored beside the local checkout
    Given an authorized checkout specification pins a Course lesson revision
    When the CLI clones that task for the first time
    Then lesson text, examples, quiz and public tests are stored locally
    And the immutable Course, Lesson, TaskInstance and Sandbox IDs are recorded
    And the local learning cache is excluded from Sandbox file sync

  @p0 @reopen
  Scenario: Reopening a lesson never destroys unfinished code
    Given a student has edited app.js in an existing local checkout
    When the student opens the same pinned lesson again
    Then the CLI reuses the existing Git checkout
    And the uncommitted changes remain unchanged
    And a different immutable revision cannot silently replace the checkout

  @p0 @sync
  Scenario: Current source is synchronized without Git commits
    Given the student edits files in VS Code or Neovim
    When the debounce interval ends
    Then the client sends only allowed working files to Sandbox
    And the latest Sandbox revision must be acknowledged before Submit
    And no background git commit or git push is performed

  @p0 @validation
  Scenario: Deterministic checks continue while AI is unavailable
    Given the AI provider or the student's selected AI budget is unavailable
    And the student has an authorized previously downloaded task
    When the student runs local public checks
    Then the IDE displays deterministic diagnostics without an AI request
    And the local result never awards Mastery

  @p0 @submit
  Scenario: Official validation is independent of AI credits
    Given a synced TaskInstance has an acknowledged Sandbox revision
    And the AI provider is unavailable
    When the student submits the task
    Then Practice Runtime uses existing Sandbox and deterministic validators
    And Student retains authority for the official progress and Mastery
    And AI Prompt is not required for the deterministic validation result

  @p0 @teacher
  Scenario: Teacher receives the latest confirmed student code
    Given the student's local code has a confirmed Sandbox revision
    When the student asks Teacher to explain an error
    Then Teacher reads only a bounded authorized snapshot of that revision
    And the conversation is stored in ms-go-dialog
    And hidden tests and other students' files are never exposed

  @p0 @security
  Scenario: A foreign student cannot clone another student's private task
    Given an HTTPS Git export belongs to another student
    When the requester tries to clone it or reuse its expired credential
    Then the server denies the request without exposing task bytes
    And the denial creates no new Student progress

  @p1 @reuse
  Scenario: Existing compatible learning material does not consume AI
    Given Planner and Course have a published compatible immutable revision
    When a second student starts the same learning target
    Then the existing material is reused according to Epic 32 contracts
    And no new lesson or task AI generation occurs

  @future @funding
  Scenario: Default funding does not constrain sharing of validated material
    Given PLATFORM_SPONSORED is the default funding mode
    And a student explicitly chooses an allowed personal provider
    And no compatible published learning material exists
    When the student accepts that generated material may be shared
    Then at most one authorized generation runs for the exact semantic request
    And the validated result follows existing Planner/Course publication rules
    And only the actual generation is charged to its recorded payer
