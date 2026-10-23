.DEFAULT_GOAL := help

PYTHON ?= python3
export PYTHONDONTWRITEBYTECODE := 1
# Export values rather than interpolating user paths into shell commands.
export QUALITY_FILES = $(FILES)
export QUALITY_BASE = $(BASE)
export QUALITY_DRY_RUN = $(DRY_RUN)
export ARTIFACT_ARCHIVE = $(ARCHIVE)

.PHONY: help install build build-image build-image-all build-bridge build-bridge-all export-artifacts import-artifacts tools test test-go test-js test-python vet syntax check validate-kit coverage coverage-go coverage-js crap crap-go crap-js metrics mutation mutation-go mutation-js mutation-all fmt tidy

## help: Show available commands (default)
help:
	@awk '/^## / { line = substr($$0, 4); pos = index(line, ":"); printf "  %-18s %s\n", substr(line, 1, pos - 1), substr(line, pos + 2) }' $(MAKEFILE_LIST)
	@printf '\nMutation scope: BASE=origin/main or FILES="path ..." (required except mutation-all).\nUse DRY_RUN=1 to discover mutants and verify baseline tests without testing mutants.\nQuote paths with spaces inside FILES, e.g. FILES="\047go-bridge/my file.go\047".\nCRAP scope is optional; omitted means all supported source files.\n'

## install: Check dependencies, build this host's environment and link ~/.local/bin/pissbox
install:
	bin/install

## build: Build the host bridge and build, save and import the host sandbox image
build: build-bridge build-image

## build-image: Build, save and import the sandbox image for this host
build-image:
	bin/build-image

## build-image-all: Build and save amd64/arm64 images without importing (uses emulation)
build-image-all:
	bin/build-image --amd64 --arm64

## build-bridge: Build the Go bridge for this host
build-bridge:
	bin/build-bridge

## build-bridge-all: Cross-compile the bridge for macOS/Linux on amd64/arm64
build-bridge-all:
	bin/build-bridge --all

## export-artifacts: Package runtime artifacts as .tar.gz (ARCHIVE=path required; needs Python 3)
export-artifacts:
	@test -n "$$ARTIFACT_ARCHIVE" || { echo 'Set ARCHIVE=path/to/artifacts.tar.gz' >&2; exit 1; }
	bin/artifacts --export "$$ARTIFACT_ARCHIVE"

## import-artifacts: Restore runtime artifacts without overwriting (ARCHIVE=path required; needs Python 3)
import-artifacts:
	@test -n "$$ARTIFACT_ARCHIVE" || { echo 'Set ARCHIVE=path/to/artifacts.tar.gz' >&2; exit 1; }
	bin/artifacts --import "$$ARTIFACT_ARCHIVE"

## tools: Install pinned quality tools under .cache (network required)
tools:
	$(PYTHON) tools/quality/run.py tools

## test: Run Go, JavaScript and Python tests
test: test-go test-js test-python

## test-go: Run Go tests with race detection and no result cache
test-go:
	cd go-bridge && go test -race -count=1 ./...

## test-js: Run extension lifecycle suites (requires installed Pi)
test-js:
	node --test 'pi-agent/extensions/**/*.test.mjs' 'tests/**/*.test.mjs'

## test-python: Run launcher, setup and quality-wrapper regression tests
test-python:
	$(PYTHON) -m unittest discover -s tests -v

## vet: Run Go vet
vet:
	cd go-bridge && go vet ./...

## syntax: Check Bash syntax
syntax:
	bash -n bin/pissbox bin/install bin/build-image bin/build-bridge sbx-kit/files/home/.pissbox/scripts/*.sh

## check: Run tests, vet, syntax and whitespace checks without modifying source
check: test vet syntax
	git diff --check

## validate-kit: Validate the kit with SBX (requires Docker sign-in)
validate-kit:
	sbx kit validate ./sbx-kit

## coverage: Generate fresh Go and JavaScript coverage reports
coverage: coverage-go coverage-js

## coverage-go: Generate Go coverage and HTML under .cache/coverage/go
coverage-go:
	$(PYTHON) tools/quality/run.py coverage-go

## coverage-js: Generate JS coverage, LCOV and HTML under .cache/coverage/js
coverage-js:
	$(PYTHON) tools/quality/run.py coverage-js

## crap: Report complexity, coverage and CRAP risk for both languages
crap: crap-go crap-js

## crap-go: Report Go CRAP risk (optional BASE or FILES)
crap-go:
	$(PYTHON) tools/quality/run.py crap-go

## crap-js: Report extension CRAP risk (optional BASE or FILES)
crap-js:
	$(PYTHON) tools/quality/run.py crap-js

## metrics: Generate fresh coverage and CRAP reports (no mutation testing)
metrics: crap

## mutation: Mutate selected Go and JS files (BASE or FILES required)
mutation: mutation-go mutation-js

## mutation-go: Mutate selected Go files; run the complete Go suite
mutation-go:
	$(PYTHON) tools/quality/run.py mutation-go

## mutation-js: Mutate selected extensions; run all extension suites
mutation-js:
	$(PYTHON) tools/quality/run.py mutation-js

## mutation-all: Deliberately mutate all supported Go and extension source files
mutation-all:
	$(PYTHON) tools/quality/run.py mutation-go --all
	$(PYTHON) tools/quality/run.py mutation-js --all

## fmt: Format Go source (modifies files)
fmt:
	cd go-bridge && go fmt ./...

## tidy: Tidy the Go module (modifies files)
tidy:
	cd go-bridge && go mod tidy -v
