---
name: "testing-rules"
description: "Readable, behavior-focused tests with KISS fixtures. Use when adding or reviewing tests."
---

## When to Use

Use when writing, reviewing, or refactoring tests.

## Procedure

1. Identify the behavior and important boundaries before writing setup.
2. Keep crucial inputs visible near each test, especially dates, ranges, and expected errors.
3. Use small local fixtures. Duplicate simple setup when it makes a test easier to read.
4. Cover the happy path, important boundaries, and meaningful failure paths.
5. Prefer focused integration tests when they exercise real behavior; avoid hollow unit tests.
6. Run focused tests first, then the relevant broader test or type-check command.

## Pitfalls

- KISS beats DRY in tests: do not hide important setup in shared helpers just to remove duplication.
- Do not maximize test count. Prefer a few high-quality cases.
- Do not reuse mocks from unrelated features or tests.
- Do not test implementation details when observable behavior is enough.
- Do not make readers trace mutations to discover the crucial mock or date values.
- Avoid fragile tests that assert irrelevant structure or exact incidental formatting.
- When few expected are in the same test, put on the top the one that will fail with most useful informations in
  reporter.

## Verification

1. Test names explain the behavior being checked.
2. A reader can see the crucial inputs and boundaries without jumping through helpers.
3. Failure output points to the behavior that broke.
4. Focused tests pass, followed by the relevant broader verification.
