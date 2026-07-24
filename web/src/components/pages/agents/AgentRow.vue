<script lang="ts" setup>
import AgentIcon from "@/components/common/agent/AgentIcon.vue";
import type { Agent } from "@/components/pages/agents";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { TableCell, TableRow } from "@/components/ui/table";
import { Ellipsis, Pen, Trash } from "@lucide/vue";
import { computed } from "vue";

const emits = defineEmits<{
  edit: [];
  delete: [];
}>();
const props = defineProps<{
  agent: Agent;
}>();

const lastActivity = computed(() => {
  if (!props.agent.lastActivityAt) return "No activity";
  return new Intl.DateTimeFormat("en", {
    day: "2-digit",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(props.agent.lastActivityAt));
});
</script>

<template>
  <TableRow>
    <TableCell class="text-muted-foreground pl-4">#{{ agent.id }}</TableCell>
    <TableCell class="min-w-56">
      <div class="flex items-center gap-3">
        <AgentIcon :value="agent.icon" class="size-7" />
        <div class="min-w-0">
          <div class="truncate font-medium leading-[120%]">{{ agent.name }}</div>
          <div class="text-sm text-muted-foreground leading-[120%]">{{ agent.kind }}</div>
        </div>
      </div>
    </TableCell>
    <TableCell class="max-w-[24rem]">
      <div v-if="agent.projects.length > 0" class="flex flex-wrap gap-1.5">
        <RouterLink
          v-for="project in agent.projects"
          :key="project.id"
          :to="{ path: '/tasks', query: { projectId: String(project.id) } }"
          class="rounded-full border px-2 py-0.5 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
        >
          {{ project.displayName }}
        </RouterLink>
      </div>
      <span v-else class="text-sm text-muted-foreground">No projects</span>
    </TableCell>
    <TableCell class="text-right">{{ agent.tasksCount }}</TableCell>
    <TableCell class="text-right">{{ agent.eventsCount }}</TableCell>
    <TableCell class="text-right text-muted-foreground">{{ lastActivity }}</TableCell>
    <TableCell class="text-right pr-4">
      <DropdownMenu>
        <DropdownMenuTrigger as-child>
          <Button size="icon-xs" class="translate-y-0.5" variant="ghost" @click.stop>
            <Ellipsis />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent @click.stop>
          <DropdownMenuItem @click.stop="emits('edit')">
            <Pen />
            Rename
          </DropdownMenuItem>
          <DropdownMenuItem variant="destructive" @click.stop="emits('delete')">
            <Trash />
            Delete
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </TableCell>
  </TableRow>
</template>
