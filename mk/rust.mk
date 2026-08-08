# =============================================================================
# Rust gate — rust/
# =============================================================================
#
# Lifted from CIC-Relay's inline Rust machinery per docs/rust-gate-extraction.md.
# The FFI targets are deliberately NOT lifted: the Go and Rust implementations
# here are independent implementations of the same specification, not two halves
# of one binary. Linking them would destroy the only reason there are two —
# that they check each other by agreeing on the vector corpus.
#
# Symmetric with mk/golang.mk by design. Same shape, same discipline.

# RUST_IMAGE_DIGEST is derived from the x-rust-version anchor in
# docker-compose.yml so the pin cannot drift between the two files.
RUST_IMAGE_DIGEST ?= $(shell grep '^x-rust-version:' docker-compose.yml | grep -oE 'sha256:[a-f0-9]{64}')

# Where cargo's caches live on the host. Outside the repository on purpose: a
# target directory is large, and this repository sits under a synced tree.
RUST_CACHE_DIR ?= $(HOME)/.cache/cic-object-model

# Line-coverage floor, to ratchet up (never down) — parity with the Go
# COVERAGE_MIN and the Python --cov-fail-under gate.
RUST_COV_MIN ?= 90

RUST_CRATE_DIR ?= rust

export RUST_CACHE_DIR

# RUST_EXEC runs a cargo command in the rust-builder. It ensures rustfmt and
# clippy are present (idempotent, cheap once installed) so every rust target is
# self-sufficient — the sibling of mk/golang.mk's GO_EXEC.
define RUST_EXEC
	docker compose exec -T rust-builder bash -eu -o pipefail -c \
		'cd /git-source/$(RUST_CRATE_DIR) && rustup component add rustfmt clippy >/dev/null 2>&1 || true; $(1)'
endef

.PHONY: rust rust.up rust.toolchain-pin rust.fmt rust.fmt-check rust.lint \
        rust.test rust.coverage rust.deny rust.release rust.quality rust.shell

# rust.toolchain-pin fails loudly when the anchor is gone.
#
# RUST_IMAGE_DIGEST is a $(shell grep ...). Against a file with no such anchor
# that yields an empty string rather than an error, and an empty image pin means
# an unpinned toolchain nobody notices — the quiet failure this ecosystem exists
# to prevent. docs/rust-gate-extraction.md names it explicitly.
rust.toolchain-pin: ## Verify the Rust image is pinned by digest
	@test -n "$(RUST_IMAGE_DIGEST)" || { \
		echo "RUST_IMAGE_DIGEST is empty: the x-rust-version anchor is missing from"; \
		echo "docker-compose.yml, so the Rust toolchain is unpinned."; exit 1; }
	@echo "Rust toolchain pinned: $(RUST_IMAGE_DIGEST)"
	@docker compose config 2>/dev/null | grep -q '$(RUST_IMAGE_DIGEST)' || { \
		echo "the resolved compose config does not carry $(RUST_IMAGE_DIGEST);"; \
		echo "the anchor is present but the service does not use it."; exit 1; }
	@echo "rust-builder resolves to the pinned digest"

# rust.up creates the cache directories before starting the container.
#
# Docker creates a missing bind source as root, and this service runs as the
# host user, so a first run would produce two root-owned directories cargo
# cannot write. Creating them here means they belong to whoever ran make.
rust.up: ## Start the rust-builder container
	@mkdir -p "$(RUST_CACHE_DIR)/cargo" "$(RUST_CACHE_DIR)/target"
	@docker compose up -d rust-builder

rust.shell: rust.up ## Interactive shell in the rust-builder
	@docker compose exec rust-builder bash

rust.fmt: rust.up ## Apply rustfmt
	@echo "--- rustfmt (write) ---"
	@$(call RUST_EXEC,cargo fmt --all)

rust.fmt-check: rust.up ## Fail if formatting differs
	@echo "--- rustfmt (check) ---"
	@$(call RUST_EXEC,cargo fmt --all --check)

rust.lint: rust.up ## clippy, warnings are errors
	@echo "--- clippy ---"
	@$(call RUST_EXEC,cargo clippy --all-targets --all-features --locked -- -D warnings)

rust.test: rust.up ## Run the test suite
	@echo "--- cargo test ---"
	@$(call RUST_EXEC,cargo test --all --locked)

rust.coverage: rust.up ## Line coverage, failing below RUST_COV_MIN
	@echo "--- cargo llvm-cov (floor $(RUST_COV_MIN)%) ---"
	@$(call RUST_EXEC,command -v cargo-llvm-cov >/dev/null 2>&1 || cargo install cargo-llvm-cov --locked)
	@$(call RUST_EXEC,rustup component add llvm-tools-preview >/dev/null 2>&1 || true)
	@$(call RUST_EXEC,cargo llvm-cov --all --locked --fail-under-lines $(RUST_COV_MIN))

rust.deny: rust.up ## Dependency advisory and licence check
	@echo "--- cargo deny ---"
	@$(call RUST_EXEC,command -v cargo-deny >/dev/null 2>&1 || cargo install cargo-deny --locked)
	@$(call RUST_EXEC,cargo deny check)

# --remap-path-prefix keeps the build reproducible: without it the container's
# /git-source path is baked into the binary and two identical sources produce
# different artifacts.
rust.release: rust.up ## Reproducible release build
	@echo "--- cargo build --release ---"
	@$(call RUST_EXEC,RUSTFLAGS="--remap-path-prefix=/git-source=." cargo build --release --locked --workspace)

rust.quality: rust.toolchain-pin rust.fmt-check rust.lint rust.coverage rust.deny ## The full Rust gate
	@echo "=== rust: gate passed ==="

rust: rust.quality
