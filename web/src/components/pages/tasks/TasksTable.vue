<script lang="ts" setup>
import type { Task } from "@/components/pages/tasks";
import TaskRow from "@/components/pages/tasks/TaskRow.vue";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { computed } from "vue";
import { ChevronsLeft, ChevronLeft, ChevronRight, ChevronsRight } from "@lucide/vue";
import type { AcceptableValue } from "reka-ui";

const props = defineProps<{
  tasks: Task[];
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
  firstPage: [];
  previousPage: [];
  nextPage: [];
  lastPage: [];
  "update:pageSize": [value: number];
}>();

const pageSizes = [10, 25, 50, 100];
const currentFrom = computed(() => (props.total === 0 ? 0 : props.offset + 1));
const currentTo = computed(() => Math.min(props.offset + props.tasks.length, props.total));
const canGoPrevious = computed(() => props.offset > 0 && !props.loading);
const canGoNext = computed(() => props.offset + props.limit < props.total && !props.loading);

function updatePageSize(value: AcceptableValue) {
  if (value === null) return;
  emits("update:pageSize", Number(value));
}
</script>

<template>
  <Table class="min-w-[58rem] table-fixed" container-class="max-h-full min-h-0 custom-scrollbar">
    <TableHeader class="sticky top-0 z-10 bg-card shadow-[0_1px_0_hsl(var(--border))]">
      <TableRow>
        <TableHead class="w-16 pl-4">ID</TableHead>
        <TableHead class="w-[24rem]">Task</TableHead>
        <TableHead class="w-44">Project</TableHead>
        <TableHead class="w-32">Status</TableHead>
        <TableHead class="w-28">Agents</TableHead>
        <TableHead class="w-36 text-right pr-4">Updated</TableHead>
      </TableRow>
    </TableHeader>
    <TableBody>
      <TableRow v-if="props.error">
        <TableCell colspan="6" class="h-24 text-center text-destructive">
          Failed to load tasks.
        </TableCell>
      </TableRow>
      <TableRow v-if="!props.loading && props.tasks.length === 0">
        <TableCell colspan="6" class="h-36 text-center">
          <div class="font-medium">No tasks yet</div>
          <div class="text-sm text-muted-foreground">
            Tasks will appear here after they are created.
          </div>
        </TableCell>
      </TableRow>
      <TaskRow v-for="task in props.tasks" :key="task.id" :task="task" />
    </TableBody>
    <TableFooter class="sticky bottom-0 z-10 bg-muted shadow-[0_-1px_0_hsl(var(--border))]">
      <TableRow>
        <TableCell colspan="6">
          <div class="flex flex-wrap items-center justify-between gap-3 py-1">
            <div class="text-sm text-muted-foreground">
              Showing {{ currentFrom }}-{{ currentTo }} of {{ props.total }} tasks
            </div>

            <div class="flex items-center gap-3">
              <label class="flex items-center gap-2 text-sm text-muted-foreground">
                Rows
                <Select
                  :model-value="String(props.pageSize)"
                  :disabled="props.loading"
                  @update:model-value="updatePageSize"
                >
                  <SelectTrigger class="h-8 w-19 bg-background" size="sm">
                    <SelectValue :placeholder="String(props.pageSize)" />
                  </SelectTrigger>
                  <SelectContent side="top">
                    <SelectItem v-for="size in pageSizes" :key="size" :value="String(size)">
                      {{ size }}
                    </SelectItem>
                  </SelectContent>
                </Select>
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
