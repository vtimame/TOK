<script lang="ts" setup>
import { useTasksQuery } from "@/api/queries/tasks.ts";
import TasksTable from "@/components/pages/tasks/TasksTable.vue";
import { useTitle } from "@vueuse/core";
import { computed, ref, watch } from "vue";

const pageSize = ref(25);
const page = ref(1);
const offset = computed(() => (page.value - 1) * pageSize.value);
const taskListParams = computed(() => ({
  limit: String(pageSize.value),
  offset: String(offset.value),
}));
const tasksQuery = useTasksQuery(taskListParams);
const tasksPage = computed(
  () =>
    tasksQuery.data.value ?? {
      tasks: [],
      total: 0,
      limit: pageSize.value,
      offset: offset.value,
    },
);
const tasks = computed(() => tasksPage.value.tasks);
const pageCount = computed(() => Math.max(1, Math.ceil(tasksPage.value.total / pageSize.value)));

watch(pageSize, () => {
  page.value = 1;
});

watch(pageCount, (nextPageCount) => {
  if (page.value > nextPageCount) {
    page.value = nextPageCount;
  }
});

useTitle("Tasks");
</script>

<template>
  <div class="mx-auto flex h-svh w-full max-w-5xl flex-col gap-4 overflow-hidden px-4 py-18">
    <div class="flex shrink-0 items-center justify-between">
      <div class="text-2xl font-bold">Tasks</div>
    </div>

    <div class="min-h-0 flex-1 overflow-hidden">
      <div
        class="flex max-h-full min-h-0 flex-col overflow-hidden rounded-lg bg-card shadow ring-1 ring-foreground/5"
      >
        <TasksTable
          :tasks="tasks"
          :total="tasksPage.total"
          :limit="tasksPage.limit"
          :offset="tasksPage.offset"
          :page="page"
          :page-count="pageCount"
          :page-size="pageSize"
          :loading="tasksQuery.isPending.value"
          :error="tasksQuery.isError.value"
          @first-page="page = 1"
          @previous-page="page = Math.max(1, page - 1)"
          @next-page="page = Math.min(pageCount, page + 1)"
          @last-page="page = pageCount"
          @update:page-size="pageSize = $event"
        />
      </div>
    </div>
  </div>
</template>
