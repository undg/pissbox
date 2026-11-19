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

## Fast, deterministic tests

- Use the cheapest interaction that preserves the behavior under test. For text input, avoid character-by-character typing unless keyboard or incremental behavior matters.
- Include only the layers needed for the assertion. Stub unrelated expensive or unstable boundaries, but never mock away the integration being tested. Experimental status alone is not a reason to mock.
- Wait for observable completion or explicitly controlled events, not sleeps or incidental exact request counts. Assert exact counts when deduplication, caching, or retries are the contract.
- Explicitly initialize background-dependent fixtures. Headless fixtures may not run the lifecycle that completes initialization in production.
- Clean up subscriptions, timers, stores, and pending work even when assertions fail. Use framework teardown or `finally`, not cleanup only at the end of the happy path.
- Investigate slow tests before increasing timeouts. Measure setup, test body, and teardown; preserve meaningful coverage rather than merely deleting slow interactions.
- Keep passing test output quiet. Capture expected logs locally and assert them when logging is part of the behavior. Restore intercepted loggers after each test. Do not blanket-suppress unexpected warnings or errors.

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
4. Changed tests pass both in isolation and with the relevant suite, not only in isolation.
5. Check for unexpected runtime increases and leaked background work.
