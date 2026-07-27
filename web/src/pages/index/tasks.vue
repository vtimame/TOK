<script lang="ts" setup>
import { toastApiError } from "@/api/axios.ts";
import { useProjectsQuery } from "@/api/queries/projects.ts";
import {
  type TaskDraft,
  useCreateTaskMutation,
  useInfiniteTasksQuery,
} from "@/api/queries/tasks.ts";
import TaskDialog from "@/components/pages/tasks/TaskDialog.vue";
import TasksTable from "@/components/pages/tasks/TasksTable.vue";
import { Button } from "@/components/ui/button";
import {
  Combobox,
  ComboboxAnchor,
  ComboboxEmpty,
  ComboboxGroup,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
  ComboboxTrigger,
} from "@/components/ui/combobox";
import { useTitle } from "@vueuse/core";
import { CheckIcon, ChevronsUpDownIcon } from "@lucide/vue";
import type { AcceptableValue } from "reka-ui";
import { computed, watch } from "vue";
import { ref } from "vue";
import { useRoute, useRouter } from "vue-router";

const route = useRoute();
const router = useRouter();
const allProjectsValue = "__all__";
const statusOptions = [
  { value: "open", label: "Open" },
  { value: "in_progress", label: "In progress" },
  { value: "blocked", label: "Blocked" },
  { value: "done", label: "Done" },
];
const selectedProjectId = ref(routeProjectId());
const selectedStatuses = ref<string[]>([]);
const showTaskDialog = ref(false);
const hasTaskRoute = computed(() => "id" in route.params);

const pageSize = ref(25);
const taskListParams = computed(() => ({
  limit: String(pageSize.value),
  projectId: selectedProjectId.value || undefined,
  status: selectedStatuses.value.length ? selectedStatuses.value.join(",") : undefined,
}));
const projectsQuery = useProjectsQuery();
const tasksQuery = useInfiniteTasksQuery(taskListParams);
const createTaskMutation = useCreateTaskMutation();
const taskPages = computed(() => tasksQuery.data.value?.pages ?? []);
const tasks = computed(() => taskPages.value.flatMap((page) => page.tasks));
const totalTasks = computed(() => taskPages.value[taskPages.value.length - 1]?.total ?? 0);
const projects = computed(() => projectsQuery.data.value?.projects ?? []);
const selectedProject = computed(() =>
  projects.value.find((project) => String(project.id) === selectedProjectId.value),
);
const selectedProjectValue = computed({
  get: () => selectedProjectId.value || allProjectsValue,
  set: (value: AcceptableValue) => {
    selectedProjectId.value = value === null || value === allProjectsValue ? "" : String(value);
  },
});
const projectFilterLabel = computed(() => selectedProject.value?.displayName || "All projects");
const statusFilterLabel = computed(() => {
  if (selectedStatuses.value.length === 0) return "All statuses";
  return selectedStatuses.value
    .map((value) => statusOptions.find((status) => status.value === value)?.label || value)
    .join(", ");
});

watch(
  () => route.query.projectId,
  () => {
    selectedProjectId.value = routeProjectId();
  },
);

watch(selectedProjectId, (projectId) => {
  const currentProjectId = routeProjectId();
  if (projectId === currentProjectId) return;

  router.replace({
    query: {
      ...route.query,
      projectId: projectId || undefined,
    },
  });
});

function routeProjectId() {
  const value = route.query.projectId;
  return Array.isArray(value) ? value[0] || "" : value || "";
}

function openTaskDialog() {
  showTaskDialog.value = true;
}

async function saveTask(input: TaskDraft) {
  const { project, ...data } = input;
  try {
    await createTaskMutation.mutateAsync({ project, data });
    showTaskDialog.value = false;
  } catch (error) {
    toastApiError(error);
  }
}

useTitle("Tasks");
</script>

<template>
  <RouterView v-if="hasTaskRoute" />

  <div v-else class="mx-auto flex h-svh w-full max-w-6xl flex-col gap-4 overflow-hidden px-4 py-18">
    <div class="flex shrink-0 items-center justify-between">
      <div class="text-2xl font-bold">Tasks</div>
      <div class="flex items-center gap-x-2">
        <Combobox v-model="selectedProjectValue">
          <ComboboxAnchor as-child>
            <ComboboxTrigger as-child>
              <Button variant="outline" class="w-48 justify-between">
                <span class="truncate">{{ projectFilterLabel }}</span>
                <ChevronsUpDownIcon class="opacity-50" />
              </Button>
            </ComboboxTrigger>
          </ComboboxAnchor>
          <ComboboxList class="w-48" align="start">
            <ComboboxInput :display-value="() => ''" placeholder="Search project..." />
            <ComboboxEmpty>No projects found.</ComboboxEmpty>
            <ComboboxGroup>
              <ComboboxItem :value="allProjectsValue">
                <div
                  class="data-[selected=true]:border-primary data-[selected=true]:bg-primary data-[selected=true]:text-primary-foreground pointer-events-none size-4 shrink-0 rounded-[4px] border transition-all select-none *:[svg]:opacity-0 data-[selected=true]:*:[svg]:opacity-100"
                  :data-selected="selectedProjectValue === allProjectsValue"
                >
                  <CheckIcon class="size-3.5 text-current" />
                </div>
                <span>All projects</span>
              </ComboboxItem>
              <ComboboxItem
                v-for="project in projects"
                :key="project.id"
                :value="String(project.id)"
              >
                <div
                  class="data-[selected=true]:border-primary data-[selected=true]:bg-primary data-[selected=true]:text-primary-foreground pointer-events-none size-4 shrink-0 rounded-[4px] border transition-all select-none *:[svg]:opacity-0 data-[selected=true]:*:[svg]:opacity-100"
                  :data-selected="selectedProjectId === String(project.id)"
                >
                  <CheckIcon class="size-3.5 text-current" />
                </div>
                <span class="truncate">{{ project.displayName }}</span>
              </ComboboxItem>
            </ComboboxGroup>
          </ComboboxList>
        </Combobox>

        <Combobox v-model="selectedStatuses" multiple>
          <ComboboxAnchor as-child>
            <ComboboxTrigger as-child>
              <Button variant="outline" class="w-42 justify-between">
                <span class="truncate">{{ statusFilterLabel }}</span>
                <ChevronsUpDownIcon class="opacity-50" />
              </Button>
            </ComboboxTrigger>
          </ComboboxAnchor>
          <ComboboxList class="w-42" align="start">
            <ComboboxInput placeholder="Search status..." />
            <ComboboxEmpty>No statuses found.</ComboboxEmpty>
            <ComboboxGroup>
              <ComboboxItem
                v-for="status in statusOptions"
                :key="status.value"
                :value="status.value"
              >
                <div
                  class="data-[selected=true]:border-primary data-[selected=true]:bg-primary data-[selected=true]:text-primary-foreground pointer-events-none size-4 shrink-0 rounded-[4px] border transition-all select-none *:[svg]:opacity-0 data-[selected=true]:*:[svg]:opacity-100"
                  :data-selected="selectedStatuses.includes(status.value)"
                >
                  <CheckIcon class="size-3.5 text-current" />
                </div>
                <span>{{ status.label }}</span>
              </ComboboxItem>
            </ComboboxGroup>
          </ComboboxList>
        </Combobox>

        <Button size="sm" @click="openTaskDialog">New task</Button>
      </div>
    </div>

    <TaskDialog
      v-model:open="showTaskDialog"
      :projects="projects"
      :project-id="selectedProjectId"
      :saving="createTaskMutation.isPending.value"
      @save="saveTask"
    />

    <div class="min-h-0 flex-1 overflow-hidden">
      <div
        class="flex max-h-full min-h-0 flex-col overflow-hidden rounded-lg bg-card shadow ring-1 ring-foreground/5"
      >
        <TasksTable
          :tasks="tasks"
          :total="totalTasks"
          :page-size="pageSize"
          :loading="tasksQuery.isPending.value"
          :fetching-more="tasksQuery.isFetchingNextPage.value"
          :has-more="tasksQuery.hasNextPage.value"
          :error="tasksQuery.isError.value"
          @load-more="tasksQuery.fetchNextPage()"
          @update:page-size="pageSize = $event"
        />
      </div>
    </div>
  </div>
</template>
