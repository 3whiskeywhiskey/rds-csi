---
phase: quick
plan: 001
type: execute
wave: 1
depends_on: []
files_modified:
  - pkg/driver/driver.go
  - pkg/utils/validation.go
autonomous: true

must_haves:
  truths:
    - "Configured volume base path is added to security allowlist at driver init"
    - "AddAllowedBasePath validates paths before adding them"
    - "Duplicate paths are handled gracefully"
  artifacts:
    - path: "pkg/utils/validation.go"
      provides: "AddAllowedBasePath function"
      exports: ["AddAllowedBasePath"]
    - path: "pkg/driver/driver.go"
      provides: "Driver initialization with base path registration"
  key_links:
    - from: "pkg/driver/driver.go"
      to: "pkg/utils/validation.go"
      via: "utils.AddAllowedBasePath call"
      pattern: "utils\\.AddAllowedBasePath"
---

<objective>
Review and commit pending changes that add dynamic base path registration to the security allowlist.

Purpose: The validation system uses a static AllowedBasePaths list, but users can configure custom base paths. These changes allow the driver to register its configured base path at initialization, ensuring custom paths pass security validation.

Output: Clean commit of security enhancement changes
</objective>

<context>
@CLAUDE.md
@pkg/utils/validation.go
@pkg/driver/driver.go
</context>

<tasks>

<task type="auto">
  <name>Task 1: Review and validate the changes</name>
  <files>pkg/utils/validation.go, pkg/driver/driver.go</files>
  <action>
    Review the pending changes:

    1. **validation.go changes** - AddAllowedBasePath() function:
       - Takes a path string, returns error
       - Returns nil for empty path (no-op)
       - Validates path via SanitizeBasePath (checks absolute, no dangerous chars)
       - Deduplicates (checks if already in list)
       - Appends to AllowedBasePaths slice
       - This is a safe, well-designed addition

    2. **driver.go changes** - Driver initialization:
       - Imports utils package
       - After logging driver info, calls utils.AddAllowedBasePath(config.RDSVolumeBasePath)
       - Returns error if AddAllowedBasePath fails
       - Logs success message
       - Properly handles error case

    Run linter and tests to confirm changes are correct:
    ```bash
    make lint
    make test
    ```
  </action>
  <verify>make verify passes (lint + test)</verify>
  <done>Changes validated as correct and safe</done>
</task>

<task type="auto">
  <name>Task 2: Commit the security enhancement</name>
  <files>pkg/utils/validation.go, pkg/driver/driver.go</files>
  <action>
    Stage and commit both files with a descriptive commit message:

    ```bash
    git add pkg/utils/validation.go pkg/driver/driver.go
    git commit -m "feat(security): add dynamic base path registration to allowlist

    Allow the driver to register its configured RDSVolumeBasePath to the
    security allowlist at initialization time. This ensures that custom
    volume base paths configured by users pass validation.

    Changes:
    - Add AddAllowedBasePath() to validation.go with proper sanitization
    - Call AddAllowedBasePath during driver init in driver.go
    - Handle duplicates gracefully, validate paths before adding"
    ```
  </action>
  <verify>git log -1 shows the new commit</verify>
  <done>Changes committed with descriptive message</done>
</task>

</tasks>

<verification>
- make verify passes
- git status shows clean working tree
- git log -1 shows proper commit message
</verification>

<success_criteria>
- Pending changes reviewed and understood
- Linter and tests pass
- Changes committed with clear commit message
- Working tree is clean
</success_criteria>

<output>
After completion, report the commit hash and confirm working tree is clean.
</output>
