# Task Sources And External Trackers

TOK's product decision is to be the local execution layer for coding agents, not
the planning source of truth for a team backlog.

External issue trackers such as GitHub Issues, Linear and Jira are expected to
remain the place where teams plan work, discuss priority, assign ownership and
manage release scope. TOK records what happened locally after work was selected:
context, claim, runs, leases, artifacts, validation evidence, overrides and
handoff state.

## Decision

TOK supports two task source modes:

- `local`: the task exists only in TOK. This is the dependency-free default for
  personal workflows, experiments and repositories that do not use an external
  tracker.
- External tracker reference: a TOK task represents the local execution record
  for an issue from a tracker such as GitHub Issues, Linear or Jira.

Local mode remains the minimum supported workflow. TOK must stay useful without
network access, hosted services, OAuth setup or tracker-specific credentials.

External tracker integration is a product direction, but it is not a requirement
for using TOK. In the alpha line, TOK should not imply full bidirectional sync
unless a specific integration implements and documents that behavior.

## External Reference Contract

The minimum external reference attached to a TOK task is:

```text
source: local | github | linear | jira
external_id
external_url
external_revision
```

The fields mean:

- `source`: where the task originated. `local` means no external tracker.
- `external_id`: the stable tracker identifier, such as a GitHub issue number,
  Linear issue key or Jira issue key.
- `external_url`: the human-readable tracker URL.
- `external_revision`: the last known tracker revision observed by TOK, such as
  an updated timestamp, ETag, version number or provider-specific sync token.

The reference identifies the upstream planning item. It does not make TOK the
owner of that item.

## State Ownership

The external tracker owns planning state:

- backlog status;
- priority and milestone;
- team assignee;
- product discussion;
- labels and triage metadata.

TOK owns local execution state:

- task claim in the local repository;
- deterministic context package;
- run lifecycle;
- lease and heartbeat;
- artifacts;
- validation evidence;
- completion evidence or override;
- handoff state.

A TOK task can be `done` even if the external issue is still open, because TOK
only knows that the local execution record has completed with evidence. The
external issue can remain open for review, release, deployment or additional
discussion.

Likewise, an external issue can be closed while a TOK task still has a failed
run. TOK preserves that failed local attempt instead of rewriting history to
match the tracker.

## Sync And Conflict States

Integrations should model sync state explicitly instead of silently overwriting
either side.

Suggested states:

```text
unlinked
linked
external_updated
local_updated
diverged
external_missing
sync_failed
```

State meanings:

- `unlinked`: a local TOK task has no external reference.
- `linked`: the external reference is known and no conflict is detected.
- `external_updated`: the tracker item changed after TOK last observed
  `external_revision`.
- `local_updated`: the TOK task changed locally and an integration may need to
  export a summary or result.
- `diverged`: both sides changed in ways that require human review.
- `external_missing`: the referenced tracker item is deleted, inaccessible or
  no longer visible to the integration.
- `sync_failed`: TOK attempted provider communication and received an error.

Conflicts should be visible in task history or integration metadata. They should
not block local execution unless a specific workflow deliberately requires a
fresh external revision before claim or completion.

## Example

```text
GitHub Issue #42: open
TOK Task: in_progress, source=github, external_id=42
TOK Run #7: failed validation
TOK Run #8: succeeded with validation artifact #31
TOK Task: done with evidence_run_id=8 and evidence_artifact_id=31
Pull Request: merged
GitHub Issue #42: still open
```

This is not inconsistent. TOK has completed the local execution record with
evidence. GitHub can still be the planning and release coordination source of
truth until a human or integration closes the issue.

Another valid state:

```text
GitHub Issue #42: closed
TOK Task: blocked
TOK Run #9: failed validation
```

TOK should keep the failed execution evidence. The closed external issue does
not erase the local audit trail.

## Unsupported In The Current Alpha

The current alpha direction does not promise:

- automatic import from GitHub, Linear or Jira;
- automatic export of TOK status back to a tracker;
- bidirectional conflict resolution;
- tracker webhooks;
- OAuth or hosted integration infrastructure;
- team-wide shared sync state;
- treating TOK as the authoritative backlog for an external tracker.

Those features can be added later as explicit integrations. They should build
on the external reference contract while preserving local mode as the default
dependency-free workflow.
