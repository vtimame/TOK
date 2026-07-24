<script lang="ts" setup>
import AgentIcon from "@/components/common/agent/AgentIcon.vue";
import { useListProjects } from "@/api/generated/hooks/useListProjects.ts";
import { useListProjectTasks } from "@/api/generated/hooks/useListProjectTasks.ts";
import { actorDisplayName, agentIconValue } from "@/api/mappers.ts";
import { useTitle } from "@vueuse/core";
import { computed } from "vue";

const projectsQuery = useListProjects();
const projects = computed(() => projectsQuery.data.value?.projects ?? []);
const selectedProject = computed(() => projects.value[0]?.name || "");
const tasksQuery = useListProjectTasks({ project: selectedProject });
const tasks = computed(() => tasksQuery.data.value?.tasks ?? []);

const projectAgents = computed(() => {
  const agents = new Map<number, { id: number; name: string; icon: string; projects: Set<string>; tasks: number }>();

  for (const project of projects.value) {
    for (const actor of project.agents ?? []) {
      const name = actorDisplayName(actor);
      const item = agents.get(actor.id) ?? {
        id: actor.id,
        name,
        icon: agentIconValue(name),
        projects: new Set<string>(),
        tasks: 0,
      };
      item.projects.add(project.display_name);
      agents.set(actor.id, item);
    }
  }

  for (const task of tasks.value) {
    for (const actor of task.agents ?? []) {
      const name = actorDisplayName(actor);
      const item = agents.get(actor.id) ?? {
        id: actor.id,
        name,
        icon: agentIconValue(name),
        projects: new Set<string>(),
        tasks: 0,
      };
      item.tasks += 1;
      agents.set(actor.id, item);
    }
  }

  return [...agents.values()].sort((a, b) => a.name.localeCompare(b.name));
});

useTitle("Agents");
</script>

<template>
  <div class="mx-auto flex h-svh w-full max-w-5xl flex-col gap-4 overflow-hidden px-4 py-18">
    <div class="flex shrink-0 items-end justify-between">
      <div>
        <div class="text-2xl font-bold">Agents</div>
        <div class="text-sm text-muted-foreground">Agents inferred from project and task history.</div>
      </div>
      <div class="text-sm text-muted-foreground">{{ projectAgents.length }} active</div>
    </div>

    <div class="min-h-0 flex-1 overflow-y-auto rounded-lg bg-card shadow ring-1 ring-foreground/5 custom-scrollbar">
      <div v-if="projectsQuery.isPending.value" class="p-6 text-sm text-muted-foreground">Loading agents...</div>
      <div v-else-if="projectsQuery.isError.value" class="p-6 text-sm text-destructive">Failed to load agents.</div>
      <div v-else-if="projectAgents.length === 0" class="p-6 text-sm text-muted-foreground">
        No agents found in project activity yet.
      </div>
      <div v-for="agent in projectAgents" v-else :key="agent.id" class="flex items-center gap-4 border-b px-4 py-3 last:border-b-0">
        <AgentIcon :value="agent.icon" class="size-8" />
        <div class="min-w-0 flex-1">
          <div class="font-medium">{{ agent.name }}</div>
          <div class="truncate text-sm text-muted-foreground">{{ [...agent.projects].join(", ") || "Task activity" }}</div>
        </div>
        <div class="text-right text-sm">
          <div class="font-medium">{{ agent.tasks }}</div>
          <div class="text-muted-foreground">tasks</div>
        </div>
      </div>
    </div>
  </div>
</template>
