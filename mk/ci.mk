# mk/ci.mk — the CI pipeline, as make targets.
#
# The workflow file must not contain steps of its own. What CI runs is what
# `make ci` runs, so a failure is reproducible locally with one command and
# nobody has to read YAML to find out what the gate actually checks.
#
# The division of labour is deliberate:
#
#   this file          WHAT runs, and in what order
#   .github/workflows  only the plumbing a runner needs — checkout, cache
#                      restore/save, artifact upload
#
# Anything that decides pass or fail belongs here.
#
# Everything runs in the dockerized builder (mk/infra.mk), so the host needs
# only docker and make. No Go, no Python, no yamllint on the host.

.PHONY: ci ci.setup ci.gates ci.deps-drift ci.spec ci.impl ci.security ci.local

# UID/GID are exported so the builder writes files the host user owns. CI used
# to set these with a raw shell step; doing it here means a local run and a CI
# run mount the same way.
export UID ?= $(shell id -u)
export GID ?= $(shell id -g)

# ci is the whole gate. CI calls exactly this, and so can you.
ci: ci.setup ci.deps-drift ci.gates ci.spec ci.impl
	@echo ""
	@echo "=== ci: all gates passed ==="

# ci.local is ci plus the things a runner does around it, for someone who wants
# the full CI experience on a laptop without pushing.
ci.local: ci
	@echo "--- Local extras (CI gets these from the runner) ---"
	@$(MAKE) infra.coverage

# ---------------------------------------------------------------------------
# Setup
# ---------------------------------------------------------------------------

# ci.setup prepares the dockerized environment. The bind-mount directories are
# created before compose runs: docker would otherwise create them as root and
# every later step would fail on permissions.
ci.setup:
	@echo "--- CI setup: bind-mount dirs, images, builder, dependencies ---"
	@mkdir -p p_venv .pip-cache
	@$(MAKE) build
	@docker compose up -d builder
	@$(MAKE) infra.deps

# ci.deps-drift fails if requirements.txt is not what requirements.in compiles
# to. A drifted lock file means CI and the developer are installing different
# things, which is the kind of difference that only shows up as a mystery.
ci.deps-drift:
	@echo "--- Dependency lock drift ---"
	@git diff --exit-code requirements.txt \
		|| { echo "requirements.txt is not in sync with requirements.in — run 'make infra.deps' and commit the result"; exit 1; }

# ---------------------------------------------------------------------------
# Gates
# ---------------------------------------------------------------------------

# ci.gates is everything that holds for this repository whether or not an
# implementation exists: integrity, links, code quality, security.
ci.gates: manifest-verify docs.link-check check ci.security

# ci.security runs the scanners as their own step rather than hiding inside
# `check`. A security finding should be legible as a security finding.
ci.security:
	@echo "--- Security scan ---"
	@$(MAKE) infra.security

# ci.spec checks that SPEC.md's normative sentences and the vector corpus have
# not drifted apart. It runs with no implementation present: it checks the
# spec/vector MAPPING, not conformance results.
ci.spec:
	@echo "--- SPEC <-> vector mapping ---"
	@docker compose exec -T builder python tools/check_spec_vectors.py

# ci.impl runs whatever implementations are present. A missing implementation
# is skipped visibly — `conformance` fails rather than passing vacuously when
# nothing is there to run the vectors.
ci.impl:
	@echo "--- Reference implementations ---"
	@if [ -f go/go.mod ]; then \
		$(MAKE) golang.quality && $(MAKE) golang.test && $(MAKE) golang.coverage-threshold; \
	else \
		echo "go/ absent — skipped"; \
	fi
	@if [ -f rust/Cargo.toml ] && [ -f mk/rust.mk ]; then \
		$(MAKE) rust; \
	else \
		echo "rust/ absent — skipped"; \
	fi
	@$(MAKE) test

# ---------------------------------------------------------------------------
# Deep verification — slower than the gate, run on demand or nightly
# ---------------------------------------------------------------------------
.PHONY: verify verify.fuzz verify.mutate

# verify is everything the gate does not have time for. Each target below is
# independently runnable during development; `make -j verify.fuzz verify.mutate`
# works because they touch different things.
verify: verify.fuzz verify.mutate verify.adversarial

# FUZZTIME is deliberately short by default so `make verify.fuzz` is usable in a
# development loop. Raise it for a real hunt: FUZZTIME=10m make verify.fuzz
FUZZTIME ?= 45s

# What this asks is not "is the output right" — the corpus answers that. It asks
# whether the library can be broken: made to panic, to hang, or to hand back
# something that is neither a canonical object nor a proper error. The sharpest
# property it checks is that the materializer's own output is accepted by its
# own validator; the two were written independently and nothing else compares
# them.
verify.fuzz:
	@echo "--- Fuzzing the materializer ($(FUZZTIME)) ---"
	@docker compose exec -T builder sh -c 'cd /app/go && \
		go test ./objectmodel/ -run FuzzMaterialize -fuzz FuzzMaterialize -fuzztime $(FUZZTIME)'
	@echo "--- Fuzzing schema-less validation ($(FUZZTIME)) ---"
	@docker compose exec -T builder sh -c 'cd /app/go && \
		go test ./objectmodel/ -run FuzzValidateCanonicalDocument -fuzz FuzzValidateCanonicalDocument -fuzztime $(FUZZTIME)'

# Breaks the code on purpose, one named change at a time, and requires the suite
# to fail. A green suite proves the code passes the tests; this proves the tests
# would notice if it did not.
verify.mutate:
	@echo "--- Mutation testing ---"
	@python3 tools/mutate.py

# Attacks INV-032 — the one invariant that asserts an impossibility — instead of
# asserting it. Part of the normal Go suite, so `make ci` runs it too; named
# here because it is worth running on its own after any change to the boundary.
.PHONY: verify.adversarial
verify.adversarial:
	@echo "--- Adversarial: can the boundary be forged, crashed or exhausted? ---"
	@docker compose exec -T builder sh -c 'cd /app/go && \
		go test ./module/ -run "TestForgery|TestNil|TestResource" -v -count=1'

# ---------------------------------------------------------------------------
# Golden expectations
# ---------------------------------------------------------------------------
.PHONY: golden.update

# Rewrites the conformance expectations from what the implementation produces,
# for when a deliberate change to the canonical form touches many at once.
#
# It does not decide whether the change is correct. It writes files; `git diff`
# is the review, and it is the only thing that makes the new expectations real.
# The run fails on purpose afterwards — a green -update would be a result nobody
# checked — and it refuses to run at all when CI is set.
golden.update:
	@echo "--- Rewriting conformance expectations from actual output ---"
	@docker compose exec -T -e CI= builder sh -c 'cd /app/go && go test ./conformance/ -count=1 -update' || true
	@echo ""
	@echo "Now read the diff. Nothing is verified until you do:"
	@git diff --stat -- conformance/ || true

# The CLI is the harness contract: bytes in, bytes out, an exit code. It is what
# a second implementation has to match, and it is checked against the corpus
# itself rather than against a separate set of golden files — a second set would
# be a second thing to keep in step, and the first time they drifted the CLI
# would be verified against a stale copy of what the vectors already say.
.PHONY: golden.cli
golden.cli:
	@echo "--- CLI contract against the corpus ---"
	@docker compose exec -T builder sh -c 'cd /app/go && go test ./cmd/... -count=1 -v'
