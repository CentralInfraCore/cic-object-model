# Makefile Cheatsheet

This file provides a comprehensive list of all available `make` commands and their functions.

## Container Lifecycle

- `make up`: Starts the `builder` development container in the background. The container will remain running until explicitly stopped.
- `make down`: Stops and removes all containers, networks, and volumes associated with the project.
- `make shell`: Opens an interactive `bash` shell inside the running `builder` container. This is the primary way to interact with the development environment.
- `make build`: Builds or rebuilds the Docker images for the `setup` and `builder` services.

## Main Development Tasks

- `make validate`: Runs the schema compiler in a validation-only mode. It checks the source schemas in `/schemas` against the rules defined in the meta-schema.
- `make test`: Executes the `pytest` test suite for the Python-based tooling. This includes running unit tests for the compiler.
- `make fmt`: Automatically formats all Python code using `black` and `isort` to ensure consistent code style.
- `make lint`: Lints the Python code with `ruff` and all YAML files with `yamllint` to catch potential errors and style issues.
- `make typecheck`: Runs static type analysis on the Python codebase using `mypy`.
- `make check`: A convenience target that runs `fmt`, `lint`, and `typecheck` in sequence.

## Release and provenance

- `make release.subject`: Prints the **release subject** — a digest over every
  tracked file except `MANIFEST.sha256` and `project.yaml`, neither of which a
  digest they carry can cover. It therefore binds `SPEC.md`, the schemas, every
  conformance vector and both implementations.
- `make release.verify`: Checks that `project.yaml`'s `buildHash` is the subject
  of the tree in front of it. Runs outside the container, stdlib only, so a
  third party can verify a release with a clone and a Python.
- `make review.check`: Checks that an external review record exists for this
  tree (INV-046). Not part of `make ci`, because it gates a release rather than
  a commit; CI runs it on pull requests into `main`.
- `make release VERSION=<version>`: The inherited Vault signing path. **Its
  descriptor handling does not yet implement INV-045** — see
  `docs/spec-defects.md` and the audit record in `reviews/`.

The previous version of this section advertised `make release-dependency` and
`make release-schema`. Neither target has ever existed in this repository: both
were inherited from the base template, and `make -n` on either returns "No rule
to make target". Anyone following this page could not begin.

## Repository Setup

- `make repo.init`: Sets up the Git hooks for this repository. Currently, this installs the `commit-msg` hook, which automatically signs commits using a local Vault agent. This should be run once after cloning the repository.

## Infrastructure & Maintenance

- `make infra.deps`: (Re)generates the `requirements.txt` file from `requirements.in` and installs all Python dependencies into the local `./p_venv` cache. Run this command after adding or removing a dependency in `requirements.in`.
- `make infra.coverage`: Generates an HTML test coverage report in the `./htmlcov` directory. This provides a detailed view of which parts of the code are covered by tests.
- `make infra.clean`: A cleanup command that stops all containers, removes all generated files (like `./p_venv`, `requirements.txt`), and deletes all caches and Docker volumes. This is useful for starting from a completely clean state.
