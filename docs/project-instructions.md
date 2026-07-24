# Project Instructions

TOK project instructions are structured agent-facing rules attached to a project.
They are part of the required handoff context, not retrieval search results.

V1 stores project-scoped instructions in the TOK database. Each instruction has:

- `scope`: currently `project`;
- `title`: short human-readable name;
- `body`: instruction text shown to agents;
- `priority`: `critical`, `high`, `normal`, or `low`;
- `enabled`: disabled instructions stay in the database but are omitted from handoff context;
- `source`: currently `manual` by default;
- timestamps for auditability.

Enabled project instructions are returned by `tok context build`, `tok context build --json`,
and the MCP `context_build` tool in priority order, then by creation order.

Future instruction scopes should be layered from broad to specific:

1. Global instructions: default rules for this TOK installation.
2. Project instructions: rules for one registered project.
3. Task or run instructions: temporary constraints for one work item.

More specific scopes may add detail or constraints, but should not silently hide broader
safety or validation rules.
