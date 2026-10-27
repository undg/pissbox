# Isolate tests by execution environment

## Goal and status

Planning only. Implementation is not authorized yet; discuss the open questions first. A fresh agent should investigate and propose answers to unresolved design questions before implementing.

`make test` should automatically test host behavior on the host and sandbox behavior inside a dedicated pissbox sandbox. It must be invocable both from the host and from an existing development sandbox, so agents retain the ability to run the full suite. No host Pi installation or manually started test sandbox should be required. Keep this dual-entry design minimal; the mechanism is not yet agreed.

## Agreed direction

- Automate image build/import, test-sandbox creation, test execution, and cleanup. Use build caching without silently testing a stale image.
- Use the real pissbox kit and sandbox environment for extension tests.
- Keep test-owned Pi configuration, state, and sandbox identity isolated from personal credentials, sessions, shared state, and existing sandboxes.
- Preserve failures and useful diagnostics; never silently skip sandbox tests or fall back to host Pi.
- Keep focused host/box targets, with `make test` orchestrating both.
- Reuse the sandbox lifecycle for future full E2E tests with a mock LLM server. The mock server is not part of this initial change.

## Repository evidence and starting points

- `Makefile` currently runs Go, JS, and Python suites directly on the host. Start with `make help` and repository `AGENTS.md`.
- `pi-agent/extensions/session-name.test.mjs` and `turn-timing.test.mjs` discover host Pi through `command -v pi`; `tests/pi-installation.mjs` resolves its API. The Homebrew fallback fails against the installed package's export restrictions. Remove the host-Pi dependency rather than merely repairing that fallback.
- Existing extension tests mostly exercise real extension implementations through simulated Pi contexts/events. Moving them inside the sandbox does not require replacing them or adding an LLM server.
- `tools/quality/README.md` documents existing Stryker, coverage, scope, isolation, and report contracts. JS messaging tests also build a Go bridge fixture; account for this test-only prerequisite rather than assuming Node and Pi alone are enough.
- Reuse existing image/launcher wrappers where safe, but do not inherit normal launcher behavior that selects personal settings, mounts shared state, or reads ambient project setup.

## Nested sandbox investigation

- Official [Linux prerequisites](https://docs.docker.com/ai/sandboxes/install/#linux) require KVM for local SBX and nested virtualization when the host is itself a VM. Installing the CLI alone is insufficient.
- The current development guest is Linux/Arm64 with a working private Docker daemon, but `/dev/kvm` is absent and no KVM modules appear in `/proc/modules`. No nested SBX launch was attempted and no packages/services were installed.
- Upstream [issue #497](https://github.com/docker/sbx-releases/issues/497) specifically requests SBX-inside-SBX support on macOS and remains open in the retrieved issue data. A [public, non-maintainer comment](https://github.com/docker/sbx-releases/issues/497#issuecomment-5697061476) reports `sbx settings set feature.sandbox-nested '{"enabled":true}'` exposing `sbx create --nested`, described as experimental nested virtualization for macOS Apple Silicon M3+ only. This is a plausible new-sandbox path, not verified SBX-inside-SBX success. The official create/settings/release-note pages checked did not mention the feature. Confirm host hardware, installed CLI help, guest KVM exposure, and inner runtime/authentication before deciding feasibility; do not change host settings or recreate existing sandboxes without approval.
- Official [development docs](https://docs.docker.com/ai/sandboxes/workflows/development/) support building images and running Docker containers inside a sandbox. That is not equivalent to nesting SBX microVMs or testing kit/VM lifecycle behavior.
- Nested Linux execution, even if made available, would test Linux-host behavior rather than macOS-host behavior. Guest-local Docker-based tests remain a possible simpler alternative, but would change the current real-SBX test requirement and need discussion.

## Mutation and test-layer requirements

- Reuse behavior-focused tests for mutation testing; no separate mutation-only suite is required. Test language need not match implementation language.
- Extension mutation tests must execute the mutated source copy inside the box, not an unchanged bundled or mounted extension. Host Go mutation tests remain host-native. For cross-boundary tests, verify which side launches the mutated implementation before trusting scores.
- Prefer one isolated sandbox per mutation invocation with isolated mutant workdirs, rather than provisioning a sandbox for every mutant. Confirm that state cannot leak between runs.
- Preserve exact-file/base-ref mutation selection and existing report locations. Map copied/in-box coverage paths back to checkout source files for CRAP/reporting. Keep fixture roots short enough for Unix sockets on both systems. Distinguish real assertion failures from broken infrastructure, stale reports, and timeouts.
- Ordinary `make test` does not run mutation testing. Future full E2E tests supplement focused tests rather than replacing them.
- Concurrency-safe sandbox/image lifecycle does not imply concurrent quality-report writers. Retain the existing prohibition on same-language concurrent coverage/mutation report writers unless report isolation is deliberately implemented.

## High-level tasks

- [ ] Classify existing tests by behavior and execution environment, including Python tests of sandbox scripts and JS tests involving the host bridge.
- [ ] Design the automated, isolated test-sandbox lifecycle and host/box entry points.
- [ ] Move existing sandbox integration tests into that environment without replacing their focused assertions with full E2E tests.
- [ ] Adapt coverage and mutation tooling to run against isolated sandbox source copies and preserve scoped reports.
- [ ] Verify automation on macOS and Linux, including failure/interruption cleanup, then update commands and documentation.

## Questions to discuss

- Which existing Python tests remain host-side fixture tests, and which behaviors also need real sandbox coverage?
- Does initial scope include a real host-to-sandbox mailbox smoke test? Moving the existing messaging suite wholly inside Linux tests local integration, not the macOS host-to-SBX boundary. A real boundary test needs a dedicated host-native daemon, isolated runtime directory and owner/registration, only the mailbox exposed to the box, and test-owned daemon cleanup. This is separate from future LLM E2E.
- How should quality-tool dependencies and test-only prerequisites be supplied without relying on host Pi or personal state? The image does not explicitly install Go, but messaging tests invoke `go build`. Define network/sign-in and host prerequisite expectations; distinguish automatic image preparation from explicitly installed quality tools, and align installer/docs if requirements change.
- What isolation and reuse boundaries keep mutation runs practical without creating a new sandbox for every mutant?
- How can guest-invoked `make test` reach the same host orchestration with minimal complexity and explicit authorization? Host-only invocation is not acceptable: agents inside pissbox must retain full-suite capability. Do not assume installed Docker/SBX provides host authority. Linux execution cannot stand in for macOS host verification. A narrow, explicitly authorized host test-runner capability is a proposal, not an agreed design; do not silently extend the messaging bridge into arbitrary host-command execution. Repository tests execute code on the host, so fixed operation names alone do not make untrusted checkout execution safe.
- Which existing contracts/docs need deliberate revision? Today `make check` promises not to build images or contact SBX, but it includes `make test`; reconcile this with the new automated lifecycle.
- How should tests receive source files, writable fixtures, and reports without exposing private checkout state? Decide whether to use a filtered source copy or narrowly scoped mounts; do not mount the whole personal Pi directory.
- How will image preparation, sandbox naming, diagnostics, and cleanup avoid collisions between concurrent invocations or interference with normal pissbox use? The build wrapper updates shared `pissbox:local` and image archives/symlinks: decide on test-owned image artifacts or serialized preparation, and image retention. Account for the optional CA secret's build-cache invalidation caveat. Cleanup must not hide the original failure.

## Test layers and acceptance direction

Running an integration test inside a real sandbox does not make it full E2E. Preserve focused tests for fast feedback and mutation testing; add a small separate E2E layer later where it proves additional behavior.

Success means one `make test` invocation prepares and runs both environments, requires no host Pi or manual sandbox startup, propagates failures, and cleans up only resources it created. Existing scoped coverage/mutation workflows must remain usable without host Pi. Exact implementation and verification commands remain to be agreed.

Verification should cover a host without Pi, source changes being exercised without stale bundled copies, missing-image preparation, a deliberately failing sandbox test, interruption, repeated/concurrent invocation, and untouched personal state. Keep host orchestration separate from the in-box test entry point to prevent recursive sandbox provisioning. Verify real SBX behavior on supported macOS/Linux hosts; report unavailable environments honestly rather than treating stubbed tests as real sandbox validation.
