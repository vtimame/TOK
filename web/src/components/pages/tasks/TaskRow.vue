<script lang="ts" setup>
import AgentIcon from "@/components/common/agent/AgentIcon.vue";
import type { Task } from "@/components/pages/tasks";
import { statusLabel, statusTone } from "@/api/mappers.ts";
import { TableCell, TableRow } from "@/components/ui/table";
import { computed } from "vue";
import { useRouter } from "vue-router";

const props = defineProps<{
  task: Task;
}>();
const router = useRouter();

const updatedAt = computed(() => {
  if (!props.task.updatedAt) return "";
  return new Intl.DateTimeFormat("en", {
    day: "2-digit",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(props.task.updatedAt));
});

function openTask() {
  router.push({
    path: `/tasks/${props.task.id}`,
    query: { projectId: String(props.task.projectId) },
  });
}
</script>

<template>
  <TableRow class="cursor-pointer" @click="openTask">
    <TableCell class="text-muted-foreground pl-4">#{{ task.id }}</TableCell>
    <TableCell class="min-w-[18rem] font-medium">
      <div class="truncate">{{ task.title }}</div>
      <div class="truncate text-sm font-normal text-muted-foreground">
        {{ task.description || "No description" }}
      </div>
    </TableCell>
    <TableCell class="max-w-40 truncate text-muted-foreground">
      {{ task.project.displayName || `#${task.projectId}` }}
    </TableCell>
    <TableCell>
      <span class="rounded-full border px-2 py-0.5 text-xs" :class="statusTone(task.status)">
        {{ statusLabel(task.status) }}
      </span>
    </TableCell>
    <TableCell>
      <div class="flex -space-x-1.5">
        <AgentIcon v-for="agent in task.agents" :key="agent" :value="agent" class="size-5" />
      </div>
      <span v-if="task.agents.length === 0" class="text-sm text-muted-foreground">No agents</span>
    </TableCell>
    <TableCell class="text-right pr-4 text-muted-foreground">{{ updatedAt }}</TableCell>
  </TableRow>
</template>
