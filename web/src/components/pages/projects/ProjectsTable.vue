<script lang="ts" setup="">
import type { Project } from "@/components/pages/projects/index.ts";
import ProjectRow from "@/components/pages/projects/ProjectRow.vue";
import {
  Table,
  TableBody,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Button } from "@/components/ui/button";
import { useUpdateProjectIndex } from "@/api/generated/hooks/useUpdateProjectIndex.ts";
import { toastApiError } from "@/api/axios.ts";
import { toast } from "vue-sonner";
import { computed } from "vue";

const props = defineProps<{
  projects: Project[];
  loading?: boolean;
  error?: boolean;
}>();

const emits = defineEmits<{
  create: [];
}>();

const updateIndexMutation = useUpdateProjectIndex({
  mutation: {
    onSuccess: (response) => {
      toast("Index updated", {
        description: `${response.project_name}: ${response.indexed_documents} documents indexed.`,
      });
    },
    onError: (error) => {
      toastApiError(error);
    },
  },
});

const totals = computed(() =>
  props.projects.reduce(
    (sum, project) => {
      sum.ready += project.taskCounts.ready;
      sum.blocked += project.taskCounts.blocked;
      sum.done += project.taskCounts.done;
      sum.total += project.taskCounts.total;
      return sum;
    },
    { ready: 0, blocked: 0, done: 0, total: 0 },
  ),
);

function showUnavailable(action: string) {
  toast(action, {
    description: "The project API does not support this action yet.",
  });
}
</script>

<template>
  <Table container-class="h-full min-h-0 custom-scrollbar">
    <TableHeader class="sticky top-0 z-10 bg-card shadow-[0_1px_0_hsl(var(--border))]">
      <TableRow>
        <TableHead class="w-16 pl-4">ID</TableHead>
        <TableHead>Project</TableHead>
        <TableHead>Path</TableHead>
        <TableHead>Status</TableHead>
        <TableHead class="text-right">Ready</TableHead>
        <TableHead class="text-right">Blocked</TableHead>
        <TableHead class="text-right">Done</TableHead>
        <TableHead class="text-right">Total</TableHead>
        <TableHead class="text-right pr-4"></TableHead>
      </TableRow>
    </TableHeader>
    <TableBody>
      <TableRow v-if="props.loading">
        <TableCell colspan="9" class="h-24 text-center text-muted-foreground">
          Loading projects...
        </TableCell>
      </TableRow>
      <TableRow v-else-if="props.error">
        <TableCell colspan="9" class="h-24 text-center text-destructive">
          Failed to load projects.
        </TableCell>
      </TableRow>
      <TableRow v-else-if="props.projects.length === 0">
        <TableCell colspan="9" class="h-36 text-center">
          <div class="flex flex-col items-center gap-3">
            <div>
              <div class="font-medium">No projects yet</div>
              <div class="text-sm text-muted-foreground">Create a project to start tracking tasks.</div>
            </div>
            <Button size="sm" @click="emits('create')">Create project</Button>
          </div>
        </TableCell>
      </TableRow>
      <ProjectRow
        v-for="project in props.projects"
        :key="project.id"
        :project="project"
        :updating-index="updateIndexMutation.isPending.value"
        @edit="showUnavailable('Rename project')"
        @delete="showUnavailable('Delete project')"
        @update-index="updateIndexMutation.mutate({ project: project.name })"
      />
    </TableBody>
    <TableFooter class="sticky bottom-0 z-10 bg-muted shadow-[0_-1px_0_hsl(var(--border))]">
      <TableRow>
        <TableCell colspan="3">
          {{ props.projects.length }} {{ props.projects.length === 1 ? "project" : "projects" }}
        </TableCell>
        <TableCell class="text-right text-muted-foreground">Totals</TableCell>
        <TableCell class="text-right">{{ totals.ready }}</TableCell>
        <TableCell class="text-right">{{ totals.blocked }}</TableCell>
        <TableCell class="text-right">{{ totals.done }}</TableCell>
        <TableCell class="text-right font-medium">{{ totals.total }}</TableCell>
        <TableCell />
      </TableRow>
    </TableFooter>
  </Table>
</template>
