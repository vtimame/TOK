# Security Policy

TOK is alpha software. It is intended for local developer workflows and is not
yet hardened as a multi-tenant service.

## Supported Versions

Security fixes apply to the current public alpha release and the `main` branch.

## Reporting a Vulnerability

Please do not report security vulnerabilities in public issues.

Use GitHub Security Advisories for the repository when available. If private
advisories are not available yet, contact the maintainers through a private
channel and include:

- a short description of the issue;
- affected command, API route, UI screen or workflow;
- reproduction steps;
- expected impact;
- whether secrets, local files or command execution are involved.

We will acknowledge reports as quickly as practical and coordinate a fix before
public disclosure.

## Security Boundaries

TOK improves workflow auditability, but it is not a sandbox.

- `tok run exec`, `tok run validate` and `tok run agent` execute local child
  processes.
- Commands run in the project workspace.
- TOK applies a pragmatic dangerous-command policy, but it does not replace
  containers, VM isolation, OS sandboxing or human code review.
- Artifacts, handoff packages, indexed source content and logs may include
  sensitive project information.
- The local HTTP UI API is intended for trusted local use.

## Data Handling

TOK stores local state in SQLite and stores run artifacts under the TOK data
directory. Operators should treat that directory as sensitive.

Validation command metadata is redacted before being written to JSON, and
secret-looking environment variables are not inherited by validation commands
by default. stdout/stderr artifact files are preserved as execution evidence
and should be reviewed before sharing.

## Before Public Deployment

Do not expose `tok ui serve` to untrusted networks without adding an
authentication and deployment model appropriate for your environment.
