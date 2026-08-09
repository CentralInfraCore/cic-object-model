# Developer Workflow

This document outlines the typical workflows for interacting with the schema framework, from initial setup to creating a new release.

## First-Time Setup

Before you begin, ensure you have the following prerequisites installed on your host machine:
- `docker`
- `docker-compose`
- `make`
- `git`

Follow these steps to initialize the project after cloning the repository:

1.  **Start the Vault Signing Agent:**
    This project requires a running Vault instance for signing release artifacts. A helper script is provided to run a temporary, local Vault server for development.

    ```sh
    # This needs to be run from the project root in a separate terminal
    ./tools/vault-sign-agent.sh -k /path/to/your/key.pem -c /path/to/your/cert.crt --root-ca-file /path/to/your/CICRootCA.crt
    ```
    This agent will remain running in the background.

2.  **Install Python Dependencies:**
    This command compiles the `requirements.in` file and installs all necessary Python packages into a local `./p_venv` directory, which is used as a cache by the Docker container.

    ```sh
    make infra.deps
    ```

3.  **Build Docker Images:**
    Build the necessary Docker images for the `setup` and `builder` services.

    ```sh
    make build
    ```

4.  **Start the Development Container:**
    This starts the `builder` container in the background.

    ```sh
    make up
    ```

5.  **Initialize Git Hooks:**
    This script sets up the `commit-msg` Git hook, which automatically signs your commits using the running Vault agent.

    ```sh
    make repo.init
    ```

Your environment is now fully configured and ready for development.

## Day-to-Day Development

This is the typical cycle you will follow when modifying or creating schemas.

1.  **Modify a Schema:**
    Make your desired changes to a schema file located in the `/schemas` directory.

2.  **Run Validation:**
    Before creating a release, it's crucial to validate your changes. The `validate` command runs the compiler in a validation-only mode.

    ```sh
    make validate
    ```

3.  **Run Tests:**
    To ensure the tooling itself is working correctly, run the `pytest` suite.

    ```sh
    make test
    ```

4.  **Commit Your Changes:**
    When you are ready, commit your changes. The `commit-msg` hook will automatically run, connect to your local Vault agent, and append a signing block to your commit message.

    ```sh
    git add .
    git commit -m "feat: Update schema with new properties"
    ```

## Creating a Release

A release here is not a compiled artifact. The subject is the normative product:
the specification, the machine-readable schemas, every conformance vector and
every implementation shipped with it (SPEC INV-045).

1.  **Ensure the working tree is clean and the gates pass.**

    ```sh
    make ci
    ```

2.  **Compute the subject and record it.**

    ```sh
    make release.subject          # prints the digest
    # write it into project.yaml's metadata.buildHash
    make manifest-update
    make release.verify           # confirms the descriptor describes this tree
    ```

3.  **Commission an external review of that subject** and file the record as
    `reviews/<subject-digest>.md`. `devel` reaches `main` only afterwards
    (INV-046); the procedure and the three commissioning prompts are in
    [`external-review.md`](../external-review.md).

    ```sh
    make review.check
    ```

4.  **Open the pull request into `main`.** CI runs `review.check` for that
    target branch, so a tree with no review for it cannot merge.

This page previously instructed `make release-dependency VERSION=v1.0.0`. That
target has never existed in this repository — it was inherited from the base
template — so the documented procedure could not be started, let alone
completed.

