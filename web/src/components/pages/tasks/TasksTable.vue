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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { computed } from "vue";
import type { AcceptableValue } from "reka-ui";
import { useInfiniteLoadTrigger } from "@/composables/useInfiniteLoadTrigger.ts";

const props = defineProps<{
  tasks: Task[];
  total: number;
  pageSize: number;
  loading?: boolean;
  fetchingMore?: boolean;
  hasMore?: boolean;
  error?: boolean;
}>();

const emits = defineEmits<{
  loadMore: [];
  "update:pageSize": [value: number];
}>();

const pageSizes = [10, 25, 50, 100];
const loadedCount = computed(() => props.tasks.length);
const canLoadMore = computed(() => Boolean(props.hasMore) && !props.error);
const loadingMore = computed(() => Boolean(props.loading || props.fetchingMore));
const { trigger: loadMoreTrigger } = useInfiniteLoadTrigger({
  hasMore: canLoadMore,
  loading: loadingMore,
  itemCount: loadedCount,
  loadMore: () => emits("loadMore"),
});

function updatePageSize(value: AcceptableValue) {
  if (value === null) return;
  emits("update:pageSize", Number(value));
}
</script>

<template>
  <Table class="table-fixed" container-class="max-h-full min-h-0 custom-scrollbar">
    <TableHeader class="sticky top-0 z-10 bg-card shadow-[0_1px_0_hsl(var(--border))]">
      <TableRow>
        <TableHead class="w-16 pl-4">ID</TableHead>
        <TableHead class="w-[22rem]">Task</TableHead>
        <TableHead class="w-40">Project</TableHead>
        <TableHead class="w-32">Status</TableHead>
        <TableHead class="w-28">Agents</TableHead>
        <TableHead class="w-32 text-right pr-4">Updated</TableHead>
      </TableRow>
    </TableHeader>
    <TableBody>
      <TableRow v-if="props.error">
        <TableCell colspan="6" class="h-24 text-center text-destructive">
          Failed to load tasks.
        </TableCell>
      </TableRow>
      <TableRow v-else-if="props.loading && props.tasks.length === 0">
        <TableCell colspan="6" class="h-24 text-center text-muted-foreground">
          Loading tasks...
        </TableCell>
      </TableRow>
      <TableRow v-else-if="props.tasks.length === 0">
        <TableCell colspan="6" class="h-36 text-center">
          <div class="font-medium">No tasks yet</div>
          <div class="text-sm text-muted-foreground">
            Tasks will appear here after they are created.
          </div>
        </TableCell>
      </TableRow>
      <TaskRow v-for="task in props.tasks" :key="task.id" :task="task" />
      <TableRow v-if="props.tasks.length > 0 && (props.hasMore || props.fetchingMore)">
        <TableCell colspan="6" class="h-10 text-center text-xs text-muted-foreground">
          <div ref="loadMoreTrigger" class="h-px w-full" />
          <span v-if="props.fetchingMore">Loading more tasks...</span>
        </TableCell>
      </TableRow>
    </TableBody>
    <TableFooter class="sticky bottom-0 z-10 bg-muted shadow-[0_-1px_0_hsl(var(--border))]">
      <TableRow>
        <TableCell colspan="6">
          <div class="flex flex-wrap items-center justify-between gap-3 py-1">
            <div class="text-sm text-muted-foreground">
              Showing {{ loadedCount }} of {{ props.total }} tasks
            </div>

            <div class="flex items-center gap-3">
              <label class="flex items-center gap-2 text-sm text-muted-foreground">
                Rows
                <Select
                  :model-value="String(props.pageSize)"
                  :disabled="props.loading || props.fetchingMore"
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
            </div>
          </div>
        </TableCell>
      </TableRow>
    </TableFooter>
  </Table>
</template>
