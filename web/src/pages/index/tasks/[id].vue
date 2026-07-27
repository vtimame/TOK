<script lang="ts" setup>
import AgentIcon from "@/components/common/agent/AgentIcon.vue";
import { toastApiError } from "@/api/axios.ts";
import {
  useBlockTaskMutation,
  useClaimTaskMutation,
  useCommentTaskMutation,
  useCompleteTaskMutation,
  useProgressTaskMutation,
  useTaskQuery,
  useUnblockTaskMutation,
} from "@/api/queries/tasks.ts";
import { statusLabel, statusTone } from "@/api/mappers.ts";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { useTitle } from "@vueuse/core";
import { computed, ref } from "vue";
import { RouterLink, useRoute } from "vue-router";

const route = useRoute<"//tasks/[id]">();
const noteBody = ref("");
const taskId = computed(() => {
  const value = route.params.id;
  return Array.isArray(value) ? value[0] : String(value || "");
});
const taskQuery = useTaskQuery(taskId);
const claimTaskMutation = useClaimTaskMutation();
const completeTaskMutation = useCompleteTaskMutation();
const commentTaskMutation = useCommentTaskMutation();
const progressTaskMutation = useProgressTaskMutation();
const blockTaskMutation = useBlockTaskMutation();
const unblockTaskMutation = useUnblockTaskMutation();

const taskDetails = computed(() => taskQuery.data.value);
const task = computed(() => taskDetails.value?.task);
const events = computed(() => taskDetails.value?.events ?? []);
const dependencies = computed(() => taskDetails.value?.dependencies ?? []);
const blockedBy = computed(() =>
  dependencies.value.filter((dependency) => dependency.role === "blocked_by"),
);
const blocks = computed(() =>
  dependencies.value.filter((dependency) => dependency.role === "blocks"),
);
const project = computed(() => task.value?.project);
const canClaim = computed(() => task.value?.status === "open");
const canComplete = computed(() => task.value?.status === "in_progress");
const notePlaceholder = computed(() => {
  if (task.value?.status === "blocked") return "Write an unblock note or add context";
  if (task.value?.status === "in_progress")
    return "Describe progress, a blocker, or completion notes";
  return "Write a comment, progress note, or blocker reason";
});
const actionPending = computed(
  () =>
    claimTaskMutation.isPending.value ||
    completeTaskMutation.isPending.value ||
    commentTaskMutation.isPending.value ||
    progressTaskMutation.isPending.value ||
    blockTaskMutation.isPending.value ||
    unblockTaskMutation.isPending.value,
);

async function claimTask() {
  if (!task.value) return;
  try {
    await claimTaskMutation.mutateAsync({
      project: task.value.project.name,
      data: { id: task.value.id },
    });
  } catch (error) {
    toastApiError(error);
  }
}

async function completeTask() {
  if (!task.value) return;
  try {
    await completeTaskMutation.mutateAsync({
      id: String(task.value.id),
      data: { note: noteBody.value.trim() || "Completed from UI." },
    });
    noteBody.value = "";
  } catch (error) {
    toastApiError(error);
  }
}

async function addNote(kind: "comment" | "progress" | "block" | "unblock") {
  if (!task.value || !noteBody.value.trim()) return;
  const body = noteBody.value.trim();

  try {
    if (kind === "comment") {
      await commentTaskMutation.mutateAsync({ id: String(task.value.id), data: { body } });
    } else if (kind === "progress") {
      await progressTaskMutation.mutateAsync({ id: String(task.value.id), data: { body } });
    } else if (kind === "block") {
      await blockTaskMutation.mutateAsync({ id: String(task.value.id), data: { reason: body } });
    } else {
      await unblockTaskMutation.mutateAsync({ id: String(task.value.id), data: { note: body } });
    }
    noteBody.value = "";
  } catch (error) {
    toastApiError(error);
  }
}

function eventBody(type: string, body: string, toStatus: string) {
  return body || statusLabel(toStatus || type);
}

function eventLabel(type: string, fromStatus: string, toStatus: string) {
  if (fromStatus || toStatus) {
    return `${statusLabel(type)} ${fromStatus ? statusLabel(fromStatus) : ""}${
      fromStatus && toStatus ? " -> " : ""
    }${toStatus ? statusLabel(toStatus) : ""}`.trim();
  }
  return statusLabel(type);
}

function formatDate(value?: string) {
  if (!value) return "";
  return new Intl.DateTimeFormat("en", {
    day: "2-digit",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

useTitle(computed(() => task.value?.title || "Task"));
</script>

<template>
  <div class="mx-auto flex h-svh w-full max-w-6xl flex-col gap-4 overflow-hidden px-4 py-18">
    <div class="flex shrink-0 items-start justify-between gap-4">
      <div class="min-w-0">
        <RouterLink
          class="text-sm text-muted-foreground hover:text-foreground"
          :to="{
            path: '/tasks',
            query: task?.projectId ? { projectId: String(task.projectId) } : {},
          }"
        >
          {{ project?.displayName || "Tasks" }}
        </RouterLink>
        <div class="truncate text-2xl font-bold">{{ task?.title || `Task #${taskId}` }}</div>
        <div class="truncate text-sm text-muted-foreground">
          Task #{{ task?.id || taskId }} in
          {{ project?.displayName || `Project #${task?.projectId || ""}` }}
        </div>
      </div>
      <span
        v-if="task"
        class="rounded-full border px-2 py-0.5 text-xs"
        :class="statusTone(task.status)"
      >
        {{ statusLabel(task.status) }}
      </span>
    </div>

    <div
      v-if="taskQuery.isPending.value"
      class="rounded-lg bg-card p-6 text-sm text-muted-foreground shadow ring-1 ring-foreground/5"
    >
      Loading task...
    </div>
    <div
      v-else-if="taskQuery.isError.value || !task"
      class="rounded-lg bg-card p-6 text-sm text-destructive shadow ring-1 ring-foreground/5"
    >
      Failed to load task.
    </div>

    <div v-else class="grid min-h-0 flex-1 grid-cols-[minmax(0,1fr)_18rem] gap-4 overflow-hidden">
      <main
        class="min-h-0 overflow-y-auto rounded-lg bg-card shadow ring-1 ring-foreground/5 custom-scrollbar"
      >
        <section class="border-b p-4">
          <div class="grid gap-5">
            <div class="grid gap-2">
              <div class="text-sm font-medium text-muted-foreground">Description</div>
              <div class="mt-1 whitespace-pre-wrap text-sm">
                {{ task.description || "No description" }}
              </div>
            </div>
            <div class="grid gap-2">
              <div class="text-sm font-medium text-muted-foreground">Acceptance criteria</div>
              <div class="mt-1 whitespace-pre-wrap text-sm">
                {{ task.acceptanceCriteria || "No acceptance criteria" }}
              </div>
            </div>
            <div class="grid gap-2">
              <div class="text-sm font-medium text-muted-foreground">Notes</div>
              <div class="mt-1 whitespace-pre-wrap text-sm">{{ task.notes || "No notes" }}</div>
            </div>
          </div>
        </section>

        <section class="border-b p-4">
          <div class="mb-3 flex items-center justify-between gap-3">
            <div class="text-sm font-medium text-muted-foreground">Add update</div>
            <div class="text-xs text-muted-foreground">{{ statusLabel(task.status) }}</div>
          </div>
          <Textarea v-model="noteBody" class="min-h-24" :placeholder="notePlaceholder" />
          <div class="mt-3 flex flex-wrap justify-end gap-2">
            <Button
              size="sm"
              variant="outline"
              :disabled="!noteBody.trim() || actionPending"
              @click="addNote('comment')"
            >
              Comment
            </Button>
            <Button
              size="sm"
              variant="outline"
              :disabled="!noteBody.trim() || actionPending"
              @click="addNote('progress')"
            >
              Progress
            </Button>
            <Button
              v-if="task.status !== 'blocked'"
              size="sm"
              variant="outline"
              :disabled="!noteBody.trim() || actionPending"
              @click="addNote('block')"
            >
              Block
            </Button>
            <Button
              v-else
              size="sm"
              variant="outline"
              :disabled="!noteBody.trim() || actionPending"
              @click="addNote('unblock')"
            >
              Unblock
            </Button>
          </div>
        </section>

        <section class="p-4">
          <div class="mb-4 flex items-center justify-between">
            <div class="text-sm font-medium text-muted-foreground">History</div>
            <div class="text-sm text-muted-foreground">{{ events.length }} events</div>
          </div>
          <div v-if="events.length === 0" class="text-sm text-muted-foreground">
            No history yet.
          </div>
          <div v-else class="space-y-0">
            <div
              v-for="event in events"
              :key="event.id"
              class="relative border-l pl-4 pb-5 last:pb-0"
            >
              <div class="absolute -left-1.5 top-1.5 size-3 rounded-full border bg-card"></div>
              <div class="flex items-start gap-3">
                <AgentIcon v-if="event.actor" :value="event.actor.icon" class="mt-0.5 size-5" />
                <div v-else class="mt-0.5 size-5 rounded-full border bg-muted"></div>
                <div class="min-w-0 flex-1">
                  <div class="flex items-center gap-2 text-sm">
                    <span class="font-medium">{{ event.actor?.name || "System" }}</span>
                    <span class="truncate text-muted-foreground">
                      {{ eventLabel(event.type, event.fromStatus, event.toStatus) }}
                    </span>
                    <span class="ml-auto shrink-0 text-xs text-muted-foreground">
                      {{ formatDate(event.createdAt) }}
                    </span>
                  </div>
                  <div class="mt-1 whitespace-pre-wrap text-sm">
                    {{ eventBody(event.type, event.body, event.toStatus) }}
                  </div>
                </div>
              </div>
            </div>
          </div>
        </section>
      </main>

      <aside
        class="min-h-0 overflow-y-auto rounded-lg bg-card p-4 shadow ring-1 ring-foreground/5 custom-scrollbar"
      >
        <div class="space-y-4">
          <div>
            <div class="mb-2 flex items-center justify-between gap-2">
              <div class="text-sm font-medium text-muted-foreground">Actions</div>
              <span
                class="rounded-full border px-2 py-0.5 text-xs"
                :class="statusTone(task.status)"
              >
                {{ statusLabel(task.status) }}
              </span>
            </div>
            <div class="mt-2 grid gap-2">
              <Button
                size="sm"
                variant="outline"
                :disabled="actionPending || !canClaim"
                @click="claimTask"
              >
                Claim
              </Button>
              <Button size="sm" :disabled="actionPending || !canComplete" @click="completeTask">
                Done
              </Button>
            </div>
          </div>

          <div>
            <div class="text-sm font-medium text-muted-foreground">Agents</div>
            <div v-if="task.agents.length" class="mt-2 flex -space-x-1.5">
              <AgentIcon v-for="agent in task.agents" :key="agent" :value="agent" class="size-6" />
            </div>
            <div v-else class="mt-2 text-sm text-muted-foreground">No agents</div>
          </div>

          <div>
            <div class="text-sm font-medium text-muted-foreground">Dependencies</div>
            <div class="mt-2 space-y-2">
              <div
                v-if="blockedBy.length === 0 && blocks.length === 0"
                class="text-sm text-muted-foreground"
              >
                No dependencies
              </div>
              <div
                v-for="dependency in blockedBy"
                :key="dependency.id"
                class="rounded-md border border-destructive/20 bg-destructive/5 p-2 text-sm"
              >
                <div class="text-muted-foreground">Blocked by</div>
                <RouterLink
                  class="font-medium hover:underline"
                  :to="{
                    path: `/tasks/${dependency.blockerTaskId}`,
                    query: { projectId: String(task.projectId) },
                  }"
                >
                  #{{ dependency.blockerTaskId }}
                </RouterLink>
              </div>
              <div
                v-for="dependency in blocks"
                :key="dependency.id"
                class="rounded-md border border-sky-500/20 bg-sky-500/5 p-2 text-sm"
              >
                <div class="text-muted-foreground">Blocks</div>
                <RouterLink
                  class="font-medium hover:underline"
                  :to="{
                    path: `/tasks/${dependency.blockedTaskId}`,
                    query: { projectId: String(task.projectId) },
                  }"
                >
                  #{{ dependency.blockedTaskId }}
                </RouterLink>
              </div>
            </div>
          </div>

          <div>
            <div class="text-sm font-medium text-muted-foreground">Metadata</div>
            <dl class="mt-2 space-y-1 text-sm">
              <div class="flex justify-between gap-2">
                <dt class="text-muted-foreground">ID</dt>
                <dd>#{{ task.id }}</dd>
              </div>
              <div class="flex justify-between gap-2">
                <dt class="text-muted-foreground">Project</dt>
                <dd class="truncate text-right">{{ task.project.displayName }}</dd>
              </div>
              <div class="flex justify-between gap-2">
                <dt class="text-muted-foreground">Created</dt>
                <dd>{{ formatDate(task.createdAt) }}</dd>
              </div>
              <div class="flex justify-between gap-2">
                <dt class="text-muted-foreground">Updated</dt>
                <dd>{{ formatDate(task.updatedAt) }}</dd>
              </div>
            </dl>
          </div>
        </div>
      </aside>
    </div>
  </div>
</template>
