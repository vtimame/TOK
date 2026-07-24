<script lang="ts" setup="">
import type { Project } from "@/components/pages/projects/index.ts";
import { TableCell, TableRow } from "@/components/ui/table";
import { Button } from "@/components/ui/button";
import { FolderSync, Pen, Trash } from "@lucide/vue";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Ellipsis } from "@lucide/vue";
import { computed } from "vue";
import { useRouter } from "vue-router";

const emits = defineEmits<{
  edit: [];
  delete: [];
  updateIndex: [];
}>();

const props = defineProps<{
  project: Project;
  updatingIndex?: boolean;
}>();
const router = useRouter();

const status = computed(() => {
  if (props.project.taskCounts.blocked > 0) return "Blocked";
  if (props.project.taskCounts.in_progress > 0) return "In progress";
  if (props.project.taskCounts.ready > 0) return "Ready";
  if (props.project.taskCounts.done === props.project.taskCounts.total && props.project.taskCounts.total > 0) {
    return "Done";
  }
  return "Open";
});

function openTasks() {
  router.push({ path: "/tasks", query: { projectId: String(props.project.id) } });
}
</script>

<template>
  <TableRow class="cursor-pointer" @click="openTasks">
    <TableCell class="text-muted-foreground pl-4">#{{ project.id }}</TableCell>
    <TableCell class="font-medium">
      {{ project.displayName }}
    </TableCell>
    <TableCell class="max-w-[22rem] truncate text-muted-foreground">{{ project.path }}</TableCell>
    <TableCell>
      <span class="rounded-full border px-2 py-0.5 text-xs text-muted-foreground">
        {{ status }}
      </span>
    </TableCell>
    <TableCell class="text-right">{{ project.taskCounts.ready }}</TableCell>
    <TableCell class="text-right">{{ project.taskCounts.blocked }}</TableCell>
    <TableCell class="text-right">{{ project.taskCounts.done }}</TableCell>
    <TableCell class="text-right font-medium">{{ project.taskCounts.total }}</TableCell>
    <TableCell class="text-right pr-4">
      <DropdownMenu>
        <DropdownMenuTrigger as-child>
          <Button size="icon-xs" class="translate-y-0.5" variant="ghost" @click.stop>
            <Ellipsis />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent @click.stop>
          <DropdownMenuItem @click.stop="emits('updateIndex')" :disabled="props.updatingIndex">
            <FolderSync />
            Update index
          </DropdownMenuItem>
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
