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
import { ChevronsLeft, ChevronLeft, ChevronRight, ChevronsRight } from "@lucide/vue";

const props = defineProps<{
  projects: Project[];
  total: number;
  limit: number;
  offset: number;
  page: number;
  pageCount: number;
  pageSize: number;
  loading?: boolean;
  error?: boolean;
}>();

const emits = defineEmits<{
  create: [];
  firstPage: [];
  previousPage: [];
  nextPage: [];
  lastPage: [];
  "update:pageSize": [value: number];
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
const pageSizes = [10, 25, 50, 100];
const currentFrom = computed(() => (props.total === 0 ? 0 : props.offset + 1));
const currentTo = computed(() => Math.min(props.offset + props.projects.length, props.total));
const canGoPrevious = computed(() => props.offset > 0 && !props.loading);
const canGoNext = computed(() => props.offset + props.limit < props.total && !props.loading);

function showUnavailable(action: string) {
  toast(action, {
    description: "The project API does not support this action yet.",
  });
}

function updatePageSize(event: Event) {
  emits("update:pageSize", Number((event.target as HTMLSelectElement).value));
}
</script>

<template>
  <Table container-class="max-h-full min-h-0 custom-scrollbar">
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
          Page totals
        </TableCell>
        <TableCell class="text-right text-muted-foreground">This page</TableCell>
        <TableCell class="text-right">{{ totals.ready }}</TableCell>
        <TableCell class="text-right">{{ totals.blocked }}</TableCell>
        <TableCell class="text-right">{{ totals.done }}</TableCell>
        <TableCell class="text-right font-medium">{{ totals.total }}</TableCell>
        <TableCell />
      </TableRow>
      <TableRow>
        <TableCell colspan="9">
          <div class="flex flex-wrap items-center justify-between gap-3 py-1">
            <div class="text-sm text-muted-foreground">
              Showing {{ currentFrom }}-{{ currentTo }} of {{ props.total }} projects
            </div>

            <div class="flex items-center gap-3">
              <label class="flex items-center gap-2 text-sm text-muted-foreground">
                Rows
                <select
                  class="h-8 rounded-md border bg-background px-2 text-sm text-foreground"
                  :value="props.pageSize"
                  :disabled="props.loading"
                  @change="updatePageSize"
                >
                  <option v-for="size in pageSizes" :key="size" :value="size">{{ size }}</option>
                </select>
              </label>

              <div class="text-sm text-muted-foreground">
                Page {{ props.page }} of {{ props.pageCount }}
              </div>

              <div class="flex items-center gap-1">
                <Button
                  size="icon-xs"
                  variant="ghost"
                  :disabled="!canGoPrevious"
                  aria-label="First page"
                  @click="emits('firstPage')"
                >
                  <ChevronsLeft />
                </Button>
                <Button
                  size="icon-xs"
                  variant="ghost"
                  :disabled="!canGoPrevious"
                  aria-label="Previous page"
                  @click="emits('previousPage')"
                >
                  <ChevronLeft />
                </Button>
                <Button
                  size="icon-xs"
                  variant="ghost"
                  :disabled="!canGoNext"
                  aria-label="Next page"
                  @click="emits('nextPage')"
                >
                  <ChevronRight />
                </Button>
                <Button
                  size="icon-xs"
                  variant="ghost"
                  :disabled="!canGoNext"
                  aria-label="Last page"
                  @click="emits('lastPage')"
                >
                  <ChevronsRight />
                </Button>
              </div>
            </div>
          </div>
        </TableCell>
      </TableRow>
    </TableFooter>
  </Table>
</template>
