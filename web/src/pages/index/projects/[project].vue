<script lang="ts" setup>
import AgentIcon from "@/components/common/agent/AgentIcon.vue";
import { Button } from "@/components/ui/button";
import { useShowProject } from "@/api/generated/hooks/useShowProject.ts";
import { useListProjectTasks } from "@/api/generated/hooks/useListProjectTasks.ts";
import { projectFromApi, statusLabel, statusTone, taskFromApi } from "@/api/mappers.ts";
import { useTitle } from "@vueuse/core";
import { computed } from "vue";
import { RouterLink, useRoute } from "vue-router";

const route = useRoute<"//projects/[project]">();
const projectName = computed(() => {
  const value = route.params.project;
  return Array.isArray(value) ? value[0] : String(value || "");
});
const projectQuery = useShowProject(
  { project: projectName },
  {
    query: {
      select: (response) => projectFromApi(response.project),
    },
  },
);
const tasksQuery = useListProjectTasks(
  { project: projectName },
  {
    query: {
      select: (response) => (response.tasks ?? []).map(taskFromApi),
    },
  },
);
const project = computed(() => projectQuery.data.value);
const tasks = computed(() => tasksQuery.data.value ?? []);

useTitle(computed(() => project.value?.displayName || "Project"));
</script>

<template>
  <div class="mx-auto flex h-svh w-full max-w-5xl flex-col gap-4 overflow-hidden px-4 py-18">
    <div class="flex shrink-0 items-start justify-between gap-4">
      <div class="min-w-0">
        <RouterLink class="text-sm text-muted-foreground hover:text-foreground" to="/projects"
          >Projects</RouterLink
        >
        <div class="truncate text-2xl font-bold">{{ project?.displayName || projectName }}</div>
        <div class="truncate text-sm text-muted-foreground">{{ project?.path }}</div>
      </div>
      <Button as-child size="sm" variant="outline">
        <RouterLink :to="{ path: '/tasks', query: { project: projectName } }"
          >Open tasks</RouterLink
        >
      </Button>
    </div>

    <div class="grid shrink-0 grid-cols-4 gap-3">
      <div class="rounded-lg bg-card p-3 shadow ring-1 ring-foreground/5">
        <div class="text-sm text-muted-foreground">Total</div>
        <div class="text-2xl font-semibold">{{ project?.taskCounts.total ?? 0 }}</div>
      </div>
      <div class="rounded-lg bg-card p-3 shadow ring-1 ring-foreground/5">
        <div class="text-sm text-muted-foreground">Ready</div>
        <div class="text-2xl font-semibold">{{ project?.taskCounts.ready ?? 0 }}</div>
      </div>
      <div class="rounded-lg bg-card p-3 shadow ring-1 ring-foreground/5">
        <div class="text-sm text-muted-foreground">Blocked</div>
        <div class="text-2xl font-semibold">{{ project?.taskCounts.blocked ?? 0 }}</div>
      </div>
      <div class="rounded-lg bg-card p-3 shadow ring-1 ring-foreground/5">
        <div class="text-sm text-muted-foreground">Done</div>
        <div class="text-2xl font-semibold">{{ project?.taskCounts.done ?? 0 }}</div>
      </div>
    </div>

    <div
      class="min-h-0 flex-1 overflow-y-auto rounded-lg bg-card shadow ring-1 ring-foreground/5 custom-scrollbar"
    >
      <div
        v-if="projectQuery.isPending.value || tasksQuery.isPending.value"
        class="p-6 text-sm text-muted-foreground"
      >
        Loading project...
      </div>
      <div
        v-else-if="projectQuery.isError.value || tasksQuery.isError.value"
        class="p-6 text-sm text-destructive"
      >
        Failed to load project.
      </div>
      <div
        v-for="task in tasks"
        v-else
        :key="task.id"
        class="flex items-center gap-4 border-b px-4 py-3 last:border-b-0"
      >
        <div class="min-w-0 flex-1">
          <div class="truncate font-medium">{{ task.title }}</div>
          <div class="truncate text-sm text-muted-foreground">
            {{ task.description || "No description" }}
          </div>
        </div>
        <div class="flex -space-x-1.5">
          <AgentIcon v-for="agent in task.agents" :key="agent" :value="agent" class="size-5" />
        </div>
        <span class="rounded-full border px-2 py-0.5 text-xs" :class="statusTone(task.status)">
          {{ statusLabel(task.status) }}
        </span>
      </div>
      <div
        v-if="!tasksQuery.isPending.value && tasks.length === 0"
        class="p-6 text-sm text-muted-foreground"
      >
        No tasks in this project.
      </div>
    </div>
  </div>
</template>
