<script lang="ts" setup>
import { statusLabel, type CompletionEvidence } from "@/api/mappers.ts";

const props = defineProps<{
  evidence: CompletionEvidence;
}>();

type SummaryRow = {
  label: string;
  value: string;
};

function completionLabel(status: CompletionEvidence["status"]) {
  switch (status) {
    case "validated":
      return "Validation evidence";
    case "missing_evidence":
      return "Missing evidence";
    case "override":
      return "Override";
    case "legacy_unknown":
      return "Legacy / unknown";
    default:
      return "Not completed";
  }
}

function completionTone(status: CompletionEvidence["status"]) {
  switch (status) {
    case "validated":
      return "border-emerald-500/30 bg-emerald-500/5";
    case "missing_evidence":
      return "border-destructive/30 bg-destructive/5";
    case "override":
      return "border-amber-500/40 bg-amber-500/10";
    case "legacy_unknown":
      return "border-border bg-muted/40";
    default:
      return "border-border bg-background";
  }
}

function summaryRows(evidence: CompletionEvidence): SummaryRow[] {
  if (
    evidence.status !== "missing_evidence" &&
    evidence.status !== "override" &&
    evidence.status !== "legacy_unknown"
  ) {
    return [];
  }
  return [
    { label: "Actor", value: evidence.actor?.name || "Unknown" },
    { label: "Completed", value: formatDate(evidence.completedAt) || "Unknown" },
  ];
}

function formatDate(value?: string) {
  if (!value) return "";
  return new Intl.DateTimeFormat("en", {
    day: "2-digit",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}
</script>

<template>
  <div class="rounded-md border p-3 text-sm" :class="completionTone(props.evidence.status)">
    <div class="flex items-center justify-between gap-3">
      <div class="font-medium">Completion evidence</div>
      <div class="shrink-0 text-xs text-muted-foreground">
        {{ completionLabel(props.evidence.status) }}
      </div>
    </div>

    <dl v-if="props.evidence.status === 'validated'" class="mt-3 grid gap-1 text-sm">
      <div class="flex justify-between gap-3">
        <dt class="text-muted-foreground">Run</dt>
        <dd class="text-right">
          #{{ props.evidence.run.id }}
          <span class="text-muted-foreground">
            {{ statusLabel(props.evidence.run.status) }}
          </span>
        </dd>
      </div>
      <div class="flex justify-between gap-3">
        <dt class="text-muted-foreground">Validation</dt>
        <dd class="text-right">
          #{{ props.evidence.artifact.id }}
          <span v-if="props.evidence.validation.status" class="text-muted-foreground">
            {{ props.evidence.validation.status }}
          </span>
        </dd>
      </div>
      <div v-if="props.evidence.validation.command" class="flex justify-between gap-3">
        <dt class="text-muted-foreground">Command</dt>
        <dd class="truncate text-right">{{ props.evidence.validation.command }}</dd>
      </div>
      <div class="flex justify-between gap-3">
        <dt class="text-muted-foreground">Actor</dt>
        <dd class="text-right">{{ props.evidence.actor?.name || "Unknown" }}</dd>
      </div>
      <div class="flex justify-between gap-3">
        <dt class="text-muted-foreground">Completed</dt>
        <dd class="text-right">{{ formatDate(props.evidence.completedAt) || "Unknown" }}</dd>
      </div>
      <div class="grid gap-1">
        <dt class="text-muted-foreground">Note</dt>
        <dd class="whitespace-pre-wrap">{{ props.evidence.note || "No note" }}</dd>
      </div>
    </dl>

    <dl v-else-if="props.evidence.status === 'missing_evidence'" class="mt-3 grid gap-1 text-sm">
      <div class="flex justify-between gap-3">
        <dt class="text-muted-foreground">Run</dt>
        <dd class="text-right">
          #{{ props.evidence.missingRunId }}
          <span v-if="props.evidence.run" class="text-muted-foreground">
            {{ statusLabel(props.evidence.run.status) }}
          </span>
        </dd>
      </div>
      <div class="flex justify-between gap-3">
        <dt class="text-muted-foreground">Validation</dt>
        <dd class="text-right">
          {{
            props.evidence.missingArtifactId
              ? `#${props.evidence.missingArtifactId}`
              : "Missing reference"
          }}
        </dd>
      </div>
      <div v-for="row in summaryRows(props.evidence)" :key="row.label" class="flex justify-between gap-3">
        <dt class="text-muted-foreground">{{ row.label }}</dt>
        <dd class="text-right">{{ row.value }}</dd>
      </div>
      <div class="grid gap-1">
        <dt class="text-muted-foreground">State</dt>
        <dd class="text-muted-foreground">Referenced completion evidence could not be found.</dd>
      </div>
    </dl>

    <dl v-else-if="props.evidence.status === 'override'" class="mt-3 grid gap-1 text-sm">
      <div v-for="row in summaryRows(props.evidence)" :key="row.label" class="flex justify-between gap-3">
        <dt class="text-muted-foreground">{{ row.label }}</dt>
        <dd class="text-right">{{ row.value }}</dd>
      </div>
      <div class="grid gap-1">
        <dt class="text-muted-foreground">Note</dt>
        <dd class="whitespace-pre-wrap">{{ props.evidence.note || "No note" }}</dd>
      </div>
      <div class="grid gap-1">
        <dt class="text-muted-foreground">Override reason</dt>
        <dd class="whitespace-pre-wrap">
          {{ props.evidence.overrideReason || "No reason recorded" }}
        </dd>
      </div>
    </dl>

    <dl v-else-if="props.evidence.status === 'legacy_unknown'" class="mt-3 grid gap-1 text-sm">
      <div v-for="row in summaryRows(props.evidence)" :key="row.label" class="flex justify-between gap-3">
        <dt class="text-muted-foreground">{{ row.label }}</dt>
        <dd class="text-right">{{ row.value }}</dd>
      </div>
      <div class="grid gap-1">
        <dt class="text-muted-foreground">State</dt>
        <dd class="text-muted-foreground">
          Completion was recorded before evidence metadata was available.
        </dd>
      </div>
    </dl>

    <div v-else class="mt-2 text-sm text-muted-foreground">
      Completion evidence will appear after the task is done.
    </div>
  </div>
</template>
