<script lang="ts" setup>
import AgentIcon from "@/components/common/agent/AgentIcon.vue";
import { Button } from "@/components/ui/button";
import type { ProjectListResponse } from "@/api/generated/models/ProjectListResponse.ts";
import { listProjectsQueryKey, useListProjects } from "@/api/generated/hooks/useListProjects.ts";
import {
  listProjectTasksQueryKey,
  useListProjectTasks,
} from "@/api/generated/hooks/useListProjectTasks.ts";
import { showTaskQueryKey, useShowTask } from "@/api/generated/hooks/useShowTask.ts";
import { useCreateTask } from "@/api/generated/hooks/useCreateTask.ts";
import { useClaimTask } from "@/api/generated/hooks/useClaimTask.ts";
import { useCompleteTask } from "@/api/generated/hooks/useCompleteTask.ts";
import { useCommentTask } from "@/api/generated/hooks/useCommentTask.ts";
import { useProgressTask } from "@/api/generated/hooks/useProgressTask.ts";
import { useBlockTask } from "@/api/generated/hooks/useBlockTask.ts";
import { useUnblockTask } from "@/api/generated/hooks/useUnblockTask.ts";
import { projectFromApi, statusLabel, statusTone, taskEventFromApi, taskFromApi } from "@/api/mappers.ts";
import { toastApiError } from "@/api/axios.ts";
import { useTitle } from "@vueuse/core";
import { useQueryClient } from "@tanstack/vue-query";
import { computed, reactive, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";

const route = useRoute();
const router = useRouter();
const queryClient = useQueryClient();

const status = ref("");
const selectedTaskId = ref<number>();
const noteBody = ref("");
const draft = reactive({
  title: "",
  description: "",
  acceptanceCriteria: "",
  notes: "",
});

const projectsQuery = useListProjects(
  { params: { limit: "100", offset: "0" } },
  {
    query: {
      select: (response: ProjectListResponse) => (response.projects ?? []).map(projectFromApi),
    },
  },
);
const projects = computed(() => projectsQuery.data.value ?? []);
const selectedProject = computed(() => {
  const queryProject = typeof route.query.project === "string" ? route.query.project : "";
  return queryProject || projects.value[0]?.name || "";
});
const taskParams = computed(() => (status.value ? { status: status.value } : {}));
const tasksQuery = useListProjectTasks(
  { project: selectedProject, params: taskParams },
  {
    query: {
      select: (response) => (response.tasks ?? []).map(taskFromApi),
    },
  },
);
const tasks = computed(() => tasksQuery.data.value ?? []);
const selectedTask = computed(() => tasks.value.find((task) => task.id === selectedTaskId.value));
const selectedTaskParam = computed(() =>
  selectedTaskId.value === undefined ? undefined : String(selectedTaskId.value),
);
const taskDetailsQuery = useShowTask(
  { id: selectedTaskParam },
  {
    query: {
      select: (response) => ({
        task: taskFromApi(response.task),
        events: (response.events ?? []).map(taskEventFromApi),
      }),
    },
  },
);
const taskEvents = computed(() => taskDetailsQuery.data.value?.events ?? []);

const createTaskMutation = useCreateTask({ mutation: { onSuccess: invalidateTaskData } });
const claimTaskMutation = useClaimTask({ mutation: { onSuccess: invalidateTaskData } });
const completeTaskMutation = useCompleteTask({ mutation: { onSuccess: invalidateTaskData } });
const commentTaskMutation = useCommentTask({ mutation: { onSuccess: invalidateSelectedTask } });
const progressTaskMutation = useProgressTask({ mutation: { onSuccess: invalidateSelectedTask } });
const blockTaskMutation = useBlockTask({ mutation: { onSuccess: invalidateTaskData } });
const unblockTaskMutation = useUnblockTask({ mutation: { onSuccess: invalidateTaskData } });

watch(
  tasks,
  (items) => {
    if (!items.length) {
      selectedTaskId.value = undefined;
      return;
    }
    if (!selectedTaskId.value || !items.some((task) => task.id === selectedTaskId.value)) {
      selectedTaskId.value = items[0].id;
    }
  },
  { immediate: true },
);

function selectProject(name: string) {
  router.replace({ query: { ...route.query, project: name || undefined } });
}

function onProjectChange(event: Event) {
  selectProject((event.target as HTMLSelectElement).value);
}

async function createTask() {
  if (!selectedProject.value || !draft.title.trim()) return;

  try {
    await createTaskMutation.mutateAsync({
      project: selectedProject.value,
      data: {
        title: draft.title.trim(),
        description: draft.description.trim() || undefined,
        acceptance_criteria: draft.acceptanceCriteria.trim() || undefined,
        notes: draft.notes.trim() || undefined,
      },
    });
    draft.title = "";
    draft.description = "";
    draft.acceptanceCriteria = "";
    draft.notes = "";
  } catch (error) {
    toastApiError(error);
  }
}

async function claimTask(id?: number) {
  if (!selectedProject.value) return;
  try {
    await claimTaskMutation.mutateAsync({ project: selectedProject.value, data: { id } });
  } catch (error) {
    toastApiError(error);
  }
}

async function completeTask() {
  if (!selectedTaskId.value) return;
  try {
    await completeTaskMutation.mutateAsync({
      id: String(selectedTaskId.value),
      data: { note: noteBody.value.trim() || "Completed from UI." },
    });
    noteBody.value = "";
  } catch (error) {
    toastApiError(error);
  }
}

async function addComment(kind: "comment" | "progress" | "block" | "unblock") {
  if (!selectedTaskId.value || !noteBody.value.trim()) return;
  const body = noteBody.value.trim();

  try {
    if (kind === "comment") {
      await commentTaskMutation.mutateAsync({ id: String(selectedTaskId.value), data: { body } });
    } else if (kind === "progress") {
      await progressTaskMutation.mutateAsync({ id: String(selectedTaskId.value), data: { body } });
    } else if (kind === "block") {
      await blockTaskMutation.mutateAsync({ id: String(selectedTaskId.value), data: { reason: body } });
    } else {
      await unblockTaskMutation.mutateAsync({ id: String(selectedTaskId.value), data: { note: body } });
    }
    noteBody.value = "";
  } catch (error) {
    toastApiError(error);
  }
}

function invalidateTaskData() {
  queryClient.invalidateQueries({ queryKey: listProjectsQueryKey() });
  queryClient.invalidateQueries({
    queryKey: listProjectTasksQueryKey({ project: selectedProject.value }, taskParams.value),
  });
  invalidateSelectedTask();
}

function invalidateSelectedTask() {
  if (selectedTaskId.value) {
    queryClient.invalidateQueries({ queryKey: showTaskQueryKey({ id: String(selectedTaskId.value) }) });
  }
}

useTitle("Tasks");
</script>

<template>
  <div class="mx-auto grid h-svh w-full max-w-5xl grid-cols-[18rem_minmax(0,1fr)] gap-4 overflow-hidden px-4 py-18">
    <aside class="flex min-h-0 flex-col rounded-lg bg-card shadow ring-1 ring-foreground/5">
      <div class="border-b p-3">
        <div class="text-lg font-semibold">Tasks</div>
        <div class="text-sm text-muted-foreground">{{ selectedProject || "No project selected" }}</div>
      </div>

      <div class="space-y-3 border-b p-3">
        <select
          class="h-9 w-full rounded-md border bg-background px-3 text-sm"
          :value="selectedProject"
          @change="onProjectChange"
        >
          <option v-for="project in projects" :key="project.id" :value="project.name">
            {{ project.displayName }}
          </option>
        </select>
        <select v-model="status" class="h-9 w-full rounded-md border bg-background px-3 text-sm">
          <option value="">All statuses</option>
          <option value="open">Open</option>
          <option value="in_progress">In progress</option>
          <option value="blocked">Blocked</option>
          <option value="done">Done</option>
        </select>
        <Button class="w-full" size="sm" variant="outline" @click="claimTask()">Claim next ready</Button>
      </div>

      <div class="min-h-0 flex-1 overflow-y-auto custom-scrollbar">
        <div v-if="tasksQuery.isPending.value" class="p-4 text-sm text-muted-foreground">Loading tasks...</div>
        <div v-else-if="tasksQuery.isError.value" class="p-4 text-sm text-destructive">Failed to load tasks.</div>
        <button
          v-for="task in tasks"
          v-else
          :key="task.id"
          class="flex w-full flex-col gap-2 border-b px-3 py-3 text-left transition-colors hover:bg-accent/50"
          :class="task.id === selectedTaskId && 'bg-accent/50'"
          @click="selectedTaskId = task.id"
        >
          <div class="flex items-start gap-2">
            <span class="min-w-0 flex-1 truncate text-sm font-medium">{{ task.title }}</span>
            <span class="rounded-full border px-2 py-0.5 text-xs" :class="statusTone(task.status)">
              {{ statusLabel(task.status) }}
            </span>
          </div>
          <div class="flex -space-x-1.5">
            <AgentIcon v-for="agent in task.agents" :key="agent" :value="agent" class="size-5" />
          </div>
        </button>
        <div v-if="!tasksQuery.isPending.value && tasks.length === 0" class="p-4 text-sm text-muted-foreground">
          No tasks for this filter.
        </div>
      </div>
    </aside>

    <main class="grid min-h-0 grid-rows-[auto_minmax(0,1fr)] gap-4">
      <section class="rounded-lg bg-card p-4 shadow ring-1 ring-foreground/5">
        <div class="mb-3 text-sm font-medium">New task</div>
        <div class="grid gap-2">
          <input v-model="draft.title" class="h-9 rounded-md border bg-background px-3 text-sm" placeholder="Title" />
          <textarea
            v-model="draft.description"
            class="min-h-20 rounded-md border bg-background px-3 py-2 text-sm"
            placeholder="Description"
          />
          <div class="grid grid-cols-2 gap-2">
            <input
              v-model="draft.acceptanceCriteria"
              class="h-9 rounded-md border bg-background px-3 text-sm"
              placeholder="Acceptance criteria"
            />
            <input v-model="draft.notes" class="h-9 rounded-md border bg-background px-3 text-sm" placeholder="Notes" />
          </div>
          <Button class="justify-self-end" size="sm" :disabled="!draft.title.trim()" @click="createTask">
            Create task
          </Button>
        </div>
      </section>

      <section class="min-h-0 overflow-hidden rounded-lg bg-card shadow ring-1 ring-foreground/5">
        <div v-if="!selectedTask" class="p-6 text-sm text-muted-foreground">Select a task to see its history.</div>
        <div v-else class="flex h-full min-h-0 flex-col">
          <div class="border-b p-4">
            <div class="flex items-start gap-3">
              <div class="min-w-0 flex-1">
                <div class="text-xl font-semibold">{{ selectedTask.title }}</div>
                <div class="mt-1 text-sm text-muted-foreground">{{ selectedTask.description || "No description" }}</div>
              </div>
              <span class="rounded-full border px-2 py-0.5 text-xs" :class="statusTone(selectedTask.status)">
                {{ statusLabel(selectedTask.status) }}
              </span>
            </div>
            <div class="mt-3 flex flex-wrap gap-2">
              <Button size="sm" variant="outline" @click="claimTask(selectedTask.id)">Claim</Button>
              <Button size="sm" variant="outline" @click="completeTask">Done</Button>
            </div>
          </div>

          <div class="min-h-0 flex-1 overflow-y-auto p-4 custom-scrollbar">
            <div v-if="taskDetailsQuery.isPending.value" class="text-sm text-muted-foreground">Loading history...</div>
            <div v-for="event in taskEvents" v-else :key="event.id" class="border-b py-3 last:border-b-0">
              <div class="flex items-center gap-2 text-sm">
                <AgentIcon v-if="event.actor" :value="event.actor.icon" class="size-5" />
                <span class="font-medium">{{ event.actor?.name || "System" }}</span>
                <span class="text-muted-foreground">{{ event.type }}</span>
              </div>
              <div class="mt-1 whitespace-pre-wrap text-sm">{{ event.body || statusLabel(event.toStatus) }}</div>
            </div>
            <div v-if="!taskDetailsQuery.isPending.value && taskEvents.length === 0" class="text-sm text-muted-foreground">
              No history yet.
            </div>
          </div>

          <div class="border-t p-3">
            <textarea
              v-model="noteBody"
              class="min-h-20 w-full rounded-md border bg-background px-3 py-2 text-sm"
              placeholder="Write an update"
            />
            <div class="mt-2 flex flex-wrap justify-end gap-2">
              <Button size="sm" variant="outline" @click="addComment('comment')">Comment</Button>
              <Button size="sm" variant="outline" @click="addComment('progress')">Progress</Button>
              <Button size="sm" variant="outline" @click="addComment('block')">Block</Button>
              <Button size="sm" variant="outline" @click="addComment('unblock')">Unblock</Button>
            </div>
          </div>
        </div>
      </section>
    </main>
  </div>
</template>
