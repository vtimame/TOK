<script lang="ts" setup>
import AgentIcon from "@/components/common/agent/AgentIcon.vue";
import { toastApiError } from "@/api/axios.ts";
import { projectFromApi, statusLabel, statusTone, taskFromApi } from "@/api/mappers.ts";
import { useGetProjectIndexIgnorePolicy } from "@/api/generated/hooks/useGetProjectIndexIgnorePolicy.ts";
import { useGetProjectIndexStatus } from "@/api/generated/hooks/useGetProjectIndexStatus.ts";
import { useListProjectTasks } from "@/api/generated/hooks/useListProjectTasks.ts";
import { useShowProject } from "@/api/generated/hooks/useShowProject.ts";
import { useUpdateProjectIndex } from "@/api/generated/hooks/useUpdateProjectIndex.ts";
import type { ListProjectTasksQueryParams } from "@/api/generated/models/ListProjectTasks.ts";
import type { ProjectResponse } from "@/api/generated/models/ProjectResponse.ts";
import type { TaskListResponse } from "@/api/generated/models/TaskListResponse.ts";
import {
  type ProjectInstructionDraft,
  useCreateProjectInstructionMutation,
  useDeleteProjectInstructionMutation,
  useDisableProjectInstructionMutation,
  useEnableProjectInstructionMutation,
  useProjectInstructionsQuery,
} from "@/api/queries/projects.ts";
import type { Task } from "@/components/pages/tasks";
import {
  ChevronLeftIcon,
  ChevronRightIcon,
  ChevronsLeftIcon,
  ChevronsRightIcon,
  PowerIcon,
  PowerOffIcon,
  Trash2Icon,
} from "@lucide/vue";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Table,
  TableBody,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import { useTitle } from "@vueuse/core";
import { computed, reactive, ref, watch } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import { toast } from "vue-sonner";
import { cn } from "@/lib/utils.ts";
import type { AcceptableValue } from "reka-ui";

const availableTabs = ["tasks", "context"] as const;
type Tab = (typeof availableTabs)[number];
type ProjectTasksPage = {
  tasks: Task[];
  total: number;
  limit: number;
  offset: number;
};

const route = useRoute<"//projects/[project]">();
const router = useRouter();

const showRuleDialog = ref(false);
const ruleDraft = reactive<ProjectInstructionDraft>({
  title: "",
  body: "",
  priority: "normal",
});

const initTab = route.query?.tab as Tab | undefined;
const tab = ref<Tab>(initTab && availableTabs.includes(initTab) ? initTab : "tasks");
const projectName = computed(() => {
  const value = route.params.project;
  return Array.isArray(value) ? value[0] : value;
});
const pageSize = ref(25);
const page = ref(1);
const offset = computed(() => (page.value - 1) * pageSize.value);
const taskListParams = computed<ListProjectTasksQueryParams>(() => ({
  limit: String(pageSize.value),
  offset: String(offset.value),
}));

const projectQuery = useShowProject(
  { project: projectName },
  {
    query: {
      select: (response: ProjectResponse) => projectFromApi(response.project),
    },
  },
);
const tasksQuery = useListProjectTasks(
  { project: projectName, params: taskListParams },
  {
    query: {
      select: (response: TaskListResponse): ProjectTasksPage => ({
        tasks: (response.tasks ?? []).map(taskFromApi),
        total: response.total,
        limit: response.limit,
        offset: response.offset,
      }),
    },
  },
);
const indexStatusQuery = useGetProjectIndexStatus({ project: projectName });
const ignorePolicyQuery = useGetProjectIndexIgnorePolicy({ project: projectName });
const instructionsQuery = useProjectInstructionsQuery(projectName);
const updateIndexMutation = useUpdateProjectIndex();
const createInstructionMutation = useCreateProjectInstructionMutation();
const enableInstructionMutation = useEnableProjectInstructionMutation();
const disableInstructionMutation = useDisableProjectInstructionMutation();
const deleteInstructionMutation = useDeleteProjectInstructionMutation();

const project = computed(() => projectQuery.data.value);
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
const instructions = computed(() => instructionsQuery.data.value ?? []);
const enabledInstructions = computed(() =>
  instructions.value.filter((instruction) => instruction.enabled),
);

const indexStatus = computed(() => indexStatusQuery.data.value);
const ignorePolicy = computed(() => ignorePolicyQuery.data.value);
const taskCounts = computed(
  () =>
    project.value?.taskCounts ?? {
      total: 0,
      ready: 0,
      open: 0,
      in_progress: 0,
      blocked: 0,
      done: 0,
    },
);
const activeCount = computed(
  () => taskCounts.value.open + taskCounts.value.in_progress + taskCounts.value.blocked,
);
const pageCount = computed(() => Math.max(1, Math.ceil(tasksPage.value.total / pageSize.value)));
const currentFrom = computed(() => (tasksPage.value.total === 0 ? 0 : tasksPage.value.offset + 1));
const currentTo = computed(() =>
  Math.min(tasksPage.value.offset + tasks.value.length, tasksPage.value.total),
);
const canGoPrevious = computed(() => tasksPage.value.offset > 0 && !tasksQuery.isPending.value);
const canGoNext = computed(
  () =>
    tasksPage.value.offset + tasksPage.value.limit < tasksPage.value.total &&
    !tasksQuery.isPending.value,
);

const pageTitle = computed(() => project.value?.displayName || projectName.value || "Project");
useTitle(pageTitle);

function formatDate(value?: string) {
  if (!value) return "";
  return new Intl.DateTimeFormat("en", {
    day: "2-digit",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

function resetRuleDraft() {
  ruleDraft.title = "";
  ruleDraft.body = "";
  ruleDraft.priority = "normal";
}

function openRuleDialog() {
  resetRuleDraft();
  showRuleDialog.value = true;
}

async function saveRule() {
  try {
    await createInstructionMutation.mutateAsync({
      project: projectName.value,
      data: {
        title: ruleDraft.title,
        body: ruleDraft.body,
        priority: ruleDraft.priority,
      },
    });
    showRuleDialog.value = false;
    toast("Rule added", { description: ruleDraft.title });
  } catch (error) {
    toastApiError(error);
  }
}

async function toggleInstruction(id: number, enabled: boolean) {
  try {
    if (enabled) {
      await disableInstructionMutation.mutateAsync({ project: projectName.value, id: String(id) });
    } else {
      await enableInstructionMutation.mutateAsync({ project: projectName.value, id: String(id) });
    }
  } catch (error) {
    toastApiError(error);
  }
}

async function removeInstruction(id: number) {
  try {
    await deleteInstructionMutation.mutateAsync({ project: projectName.value, id: String(id) });
    toast("Rule removed");
  } catch (error) {
    toastApiError(error);
  }
}

async function updateIndex() {
  try {
    const result = await updateIndexMutation.mutateAsync({ project: projectName.value });
    await indexStatusQuery.refetch();
    toast("Index updated", {
      description: `${result.indexed_documents} documents, ${result.indexed_chunks} chunks.`,
    });
  } catch (error) {
    toastApiError(error);
  }
}

function updatePageSize(value: AcceptableValue) {
  if (value === null) return;
  pageSize.value = Number(value);
}

watch(pageSize, () => {
  page.value = 1;
});

watch(projectName, () => {
  page.value = 1;
});

watch(pageCount, (nextPageCount) => {
  if (page.value > nextPageCount) {
    page.value = nextPageCount;
  }
});

watch(tab, (v: Tab) => router.replace({ query: { ...route.query, tab: v } }));
</script>

<template>
  <div class="mx-auto flex h-svh w-full max-w-5xl flex-col gap-4 overflow-hidden px-4 py-18">
    <Dialog v-model:open="showRuleDialog">
      <DialogContent class="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Add context rule</DialogTitle>
          <DialogDescription
            >Project rules are included in agent handoff context.</DialogDescription
          >
        </DialogHeader>
        <div class="grid gap-4">
          <div class="grid gap-2">
            <Label for="rule-title">Title</Label>
            <Input id="rule-title" v-model="ruleDraft.title" placeholder="Documentation lookup" />
          </div>
          <div class="grid gap-2">
            <Label for="rule-body">Body</Label>
            <Textarea
              id="rule-body"
              v-model="ruleDraft.body"
              class="min-h-28"
              placeholder="Use Context7 before changing code that depends on external libraries."
            />
          </div>
          <div class="grid gap-2">
            <Label for="rule-priority">Priority</Label>
            <Select
              :model-value="ruleDraft.priority"
              @update:model-value="ruleDraft.priority = String($event || 'normal')"
            >
              <SelectTrigger id="rule-priority" class="w-full">
                <SelectValue placeholder="Normal" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="critical">Critical</SelectItem>
                <SelectItem value="high">High</SelectItem>
                <SelectItem value="normal">Normal</SelectItem>
                <SelectItem value="low">Low</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>
        <DialogFooter>
          <Button
            variant="outline"
            :disabled="createInstructionMutation.isPending.value"
            @click="showRuleDialog = false"
          >
            Cancel
          </Button>
          <Button
            :disabled="
              createInstructionMutation.isPending.value ||
              !ruleDraft.title?.trim() ||
              !ruleDraft.body?.trim()
            "
            @click="saveRule"
          >
            Add rule
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <div class="flex shrink-0 flex-wrap items-start justify-between gap-3">
      <div class="min-w-0">
        <div class="flex flex-wrap items-center gap-2">
          <div class="truncate text-2xl font-bold">{{ project?.displayName || projectName }}</div>
        </div>
        <div class="mt-1 max-w-3xl truncate text-sm text-muted-foreground">
          {{ project?.path || "Loading project..." }}
        </div>
      </div>

      <div class="flex items-center gap-x-2">
        <Button as-child size="sm" variant="outline">
          <RouterLink
            :to="{
              path: '/tasks',
              query: { projectId: project?.id ? String(project.id) : undefined },
            }"
          >
            Open tasks
          </RouterLink>
        </Button>
        <Button size="sm" @click="openRuleDialog"> Add context rule </Button>
      </div>
    </div>

    <div class="grid shrink-0 grid-cols-2 gap-3 md:grid-cols-4">
      <div class="rounded-lg bg-card p-3 shadow ring-1 ring-foreground/5">
        <div class="text-xs font-medium text-muted-foreground">Ready</div>
        <div class="mt-1 text-2xl font-semibold">{{ taskCounts.ready }}</div>
      </div>
      <div class="rounded-lg bg-card p-3 shadow ring-1 ring-foreground/5">
        <div class="text-xs font-medium text-muted-foreground">Active</div>
        <div class="mt-1 text-2xl font-semibold">{{ activeCount }}</div>
      </div>
      <div class="rounded-lg bg-card p-3 shadow ring-1 ring-foreground/5">
        <div class="text-xs font-medium text-muted-foreground">Blocked</div>
        <div class="mt-1 text-2xl font-semibold">{{ taskCounts.blocked }}</div>
      </div>
      <div class="rounded-lg bg-card p-3 shadow ring-1 ring-foreground/5">
        <div class="text-xs font-medium text-muted-foreground">Done</div>
        <div class="mt-1 text-2xl font-semibold">{{ taskCounts.done }}</div>
      </div>
    </div>

    <Tabs v-model="tab" class="flex min-h-0 flex-1 flex-col gap-3">
      <TabsList class="w-fit">
        <TabsTrigger value="tasks">Tasks</TabsTrigger>
        <TabsTrigger value="context">Context & rules</TabsTrigger>
      </TabsList>

      <TabsContent value="tasks" class="min-h-0 flex-1 overflow-hidden">
        <div
          class="flex max-h-full min-h-0 flex-col overflow-hidden rounded-lg bg-card shadow ring-1 ring-foreground/5"
        >
          <Table
            class="min-w-[48rem] table-fixed"
            container-class="max-h-full min-h-0 custom-scrollbar"
          >
            <TableHeader class="sticky top-0 z-10 bg-card shadow-[0_1px_0_hsl(var(--border))]">
              <TableRow>
                <TableHead class="w-16 pl-4">ID</TableHead>
                <TableHead class="w-[24rem]">Task</TableHead>
                <TableHead class="w-32">Status</TableHead>
                <TableHead class="w-28">Agents</TableHead>
                <TableHead class="w-36 text-right pr-4">Updated</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-if="tasksQuery.isError.value">
                <TableCell colspan="5" class="h-24 text-center text-destructive">
                  Failed to load tasks.
                </TableCell>
              </TableRow>
              <TableRow v-else-if="tasksQuery.isPending.value">
                <TableCell colspan="5" class="h-24 text-center text-muted-foreground">
                  Loading tasks...
                </TableCell>
              </TableRow>
              <TableRow v-else-if="tasks.length === 0">
                <TableCell colspan="5" class="h-36 text-center">
                  <div class="font-medium">No tasks in this project</div>
                  <div class="text-sm text-muted-foreground">
                    New project work will appear here.
                  </div>
                </TableCell>
              </TableRow>
              <template v-else>
                <TableRow
                  v-for="task in tasks"
                  :key="task.id"
                  class="cursor-pointer"
                  @click="
                    $router.push({
                      path: `/tasks/${task.id}`,
                      query: { projectId: String(task.projectId) },
                    })
                  "
                >
                  <TableCell class="pl-4 text-muted-foreground">#{{ task.id }}</TableCell>
                  <TableCell class="w-[24rem] max-w-[24rem]">
                    <div class="truncate font-medium">{{ task.title }}</div>
                    <div class="truncate text-sm text-muted-foreground">
                      {{ task.description || "No description" }}
                    </div>
                  </TableCell>
                  <TableCell class="w-32">
                    <span
                      class="rounded-full border px-2 py-0.5 text-xs"
                      :class="statusTone(task.status)"
                    >
                      {{ statusLabel(task.status) }}
                    </span>
                  </TableCell>
                  <TableCell class="w-28">
                    <div v-if="task.agents.length" class="flex -space-x-1.5">
                      <AgentIcon
                        v-for="agent in task.agents"
                        :key="agent"
                        :value="agent"
                        class="size-5"
                      />
                    </div>
                    <span v-else class="text-sm text-muted-foreground">No agents</span>
                  </TableCell>
                  <TableCell class="w-36 pr-4 text-right text-muted-foreground">{{
                    formatDate(task.updatedAt)
                  }}</TableCell>
                </TableRow>
              </template>
            </TableBody>
            <TableFooter class="sticky bottom-0 z-10 bg-muted shadow-[0_-1px_0_hsl(var(--border))]">
              <TableRow>
                <TableCell colspan="5">
                  <div class="flex flex-wrap items-center justify-between gap-3 py-1">
                    <div class="text-sm text-muted-foreground">
                      Showing {{ currentFrom }}-{{ currentTo }} of {{ tasksPage.total }} tasks
                    </div>

                    <div class="flex items-center gap-3">
                      <label class="flex items-center gap-2 text-sm text-muted-foreground">
                        Rows
                        <Select
                          :model-value="String(pageSize)"
                          :disabled="tasksQuery.isPending.value"
                          @update:model-value="updatePageSize"
                        >
                          <SelectTrigger class="h-8 w-19 bg-background" size="sm">
                            <SelectValue :placeholder="String(pageSize)" />
                          </SelectTrigger>
                          <SelectContent side="top">
                            <SelectItem value="10">10</SelectItem>
                            <SelectItem value="25">25</SelectItem>
                            <SelectItem value="50">50</SelectItem>
                            <SelectItem value="100">100</SelectItem>
                          </SelectContent>
                        </Select>
                      </label>

                      <div class="text-sm text-muted-foreground">
                        Page {{ page }} of {{ pageCount }}
                      </div>

                      <div class="flex items-center gap-1">
                        <Button
                          size="icon-xs"
                          variant="ghost"
                          :disabled="!canGoPrevious"
                          aria-label="First page"
                          @click="page = 1"
                        >
                          <ChevronsLeftIcon />
                        </Button>
                        <Button
                          size="icon-xs"
                          variant="ghost"
                          :disabled="!canGoPrevious"
                          aria-label="Previous page"
                          @click="page = Math.max(1, page - 1)"
                        >
                          <ChevronLeftIcon />
                        </Button>
                        <Button
                          size="icon-xs"
                          variant="ghost"
                          :disabled="!canGoNext"
                          aria-label="Next page"
                          @click="page = Math.min(pageCount, page + 1)"
                        >
                          <ChevronRightIcon />
                        </Button>
                        <Button
                          size="icon-xs"
                          variant="ghost"
                          :disabled="!canGoNext"
                          aria-label="Last page"
                          @click="page = pageCount"
                        >
                          <ChevronsRightIcon />
                        </Button>
                      </div>
                    </div>
                  </div>
                </TableCell>
              </TableRow>
            </TableFooter>
          </Table>
        </div>
      </TabsContent>

      <TabsContent value="context" class="min-h-0 flex-1 overflow-y-auto custom-scrollbar">
        <div class="grid gap-3 lg:grid-cols-[minmax(0,1fr)_22rem]">
          <div class="space-y-3">
            <div class="rounded-lg bg-card shadow ring-1 ring-foreground/5">
              <div class="flex items-center justify-between gap-3 border-b px-4 py-3">
                <div>
                  <div class="font-medium">Agent rules</div>
                  <div class="text-sm text-muted-foreground">
                    {{ enabledInstructions.length }} enabled
                  </div>
                </div>
                <Button size="sm" variant="outline" @click="openRuleDialog"> Add rule </Button>
              </div>

              <div
                v-if="instructionsQuery.isPending.value"
                class="p-6 text-sm text-muted-foreground"
              >
                Loading rules...
              </div>
              <div v-else-if="instructionsQuery.isError.value" class="p-6 text-sm text-destructive">
                Failed to load rules.
              </div>
              <div v-else-if="instructions.length === 0" class="p-6 text-sm text-muted-foreground">
                No rules configured.
              </div>
              <div v-else class="divide-y">
                <div
                  v-for="instruction in instructions"
                  :key="instruction.id"
                  class="grid gap-3 px-4 py-3 sm:grid-cols-[minmax(0,1fr)_auto]"
                  :class="!instruction.enabled && 'opacity-55'"
                >
                  <div class="min-w-0">
                    <div class="flex flex-wrap items-center gap-2">
                      <div class="truncate font-medium">{{ instruction.title }}</div>
                      <Badge variant="outline">{{ instruction.priority }}</Badge>
                      <Badge v-if="!instruction.enabled" variant="secondary">Disabled</Badge>
                    </div>
                    <p class="mt-1 line-clamp-3 text-sm text-muted-foreground">
                      {{ instruction.body }}
                    </p>
                    <div class="mt-2 text-xs text-muted-foreground">
                      Updated {{ formatDate(instruction.updatedAt) }}
                    </div>
                  </div>
                  <div class="flex items-start gap-1">
                    <Button
                      size="icon-xs"
                      variant="ghost"
                      :aria-label="instruction.enabled ? 'Disable rule' : 'Enable rule'"
                      @click="toggleInstruction(instruction.id, instruction.enabled)"
                    >
                      <PowerOffIcon v-if="instruction.enabled" />
                      <PowerIcon v-else />
                    </Button>
                    <Button
                      size="icon-xs"
                      variant="ghost"
                      aria-label="Remove rule"
                      @click="removeInstruction(instruction.id)"
                    >
                      <Trash2Icon />
                    </Button>
                  </div>
                </div>
              </div>
            </div>

            <div class="rounded-lg bg-card shadow ring-1 ring-foreground/5">
              <div class="border-b px-4 py-3">
                <div class="font-medium">Ignore policy</div>
                <div class="text-sm text-muted-foreground">
                  {{ ignorePolicy?.ignore_patterns?.length ?? 0 }} patterns
                </div>
              </div>
              <div
                v-if="ignorePolicyQuery.isPending.value"
                class="p-6 text-sm text-muted-foreground"
              >
                Loading ignore policy...
              </div>
              <div v-else-if="ignorePolicyQuery.isError.value" class="p-6 text-sm text-destructive">
                Failed to load ignore policy.
              </div>
              <div v-else class="flex flex-wrap gap-2 p-4">
                <Badge
                  v-for="pattern in ignorePolicy?.ignore_patterns ?? []"
                  :key="pattern"
                  variant="outline"
                  class="font-mono"
                >
                  {{ pattern }}
                </Badge>
                <span
                  v-if="(ignorePolicy?.ignore_patterns ?? []).length === 0"
                  class="text-sm text-muted-foreground"
                >
                  No ignore patterns.
                </span>
              </div>
            </div>
          </div>

          <div class="space-y-3">
            <div class="rounded-lg bg-card p-4 shadow ring-1 ring-foreground/5">
              <div class="flex items-center justify-between gap-3">
                <div>
                  <div class="font-medium">Index</div>
                  <div class="text-sm text-muted-foreground">
                    {{
                      indexStatus?.updated_at ? formatDate(indexStatus.updated_at) : "Never indexed"
                    }}
                  </div>
                </div>
                <Badge variant="outline" class="capitalize">{{
                  indexStatus?.state || "unknown"
                }}</Badge>
              </div>
              <Separator class="my-4" />
              <div class="grid grid-cols-2 gap-3 text-sm">
                <div>
                  <div class="text-muted-foreground">Documents</div>
                  <div class="text-lg font-semibold">{{ indexStatus?.indexed_documents ?? 0 }}</div>
                </div>
                <div>
                  <div class="text-muted-foreground">Chunks</div>
                  <div class="text-lg font-semibold">{{ indexStatus?.indexed_chunks ?? 0 }}</div>
                </div>
                <div>
                  <div class="text-muted-foreground">Skipped</div>
                  <div class="text-lg font-semibold">{{ indexStatus?.skipped_files ?? 0 }}</div>
                </div>
                <div>
                  <div class="text-muted-foreground">Path</div>
                  <div class="flex items-center gap-1.5 text-lg font-semibold">
                    <span
                      :class="cn(indexStatus?.path_exists ? 'text-green-400' : 'text-destructive')"
                      >{{ indexStatus?.path_exists ? "OK" : "Missing" }}</span
                    >
                  </div>
                </div>
              </div>
              <Button
                class="mt-4 w-full"
                size="sm"
                variant="outline"
                :disabled="updateIndexMutation.isPending.value"
                @click="updateIndex"
              >
                Update index
              </Button>
            </div>

            <div class="rounded-lg bg-card p-4 shadow ring-1 ring-foreground/5">
              <div class="mb-3 flex items-center gap-2">
                <div class="font-medium">Context readiness</div>
              </div>
              <div class="space-y-3 text-sm">
                <div class="flex items-center justify-between gap-3">
                  <span class="text-muted-foreground">Lexical index</span>
                  <Badge :variant="indexStatus?.state === 'ready' ? 'default' : 'outline'">
                    {{ indexStatus?.state === "ready" ? "Ready" : "Check" }}
                  </Badge>
                </div>
                <div class="flex items-center justify-between gap-3">
                  <span class="text-muted-foreground">Agent rules</span>
                  <Badge :variant="enabledInstructions.length ? 'default' : 'outline'">
                    {{ enabledInstructions.length }}
                  </Badge>
                </div>
                <div class="flex items-center justify-between gap-3">
                  <span class="text-muted-foreground">Ignore policy</span>
                  <Badge
                    :variant="(ignorePolicy?.ignore_patterns ?? []).length ? 'default' : 'outline'"
                  >
                    {{ (ignorePolicy?.ignore_patterns ?? []).length }}
                  </Badge>
                </div>
              </div>
            </div>

            <div class="rounded-lg bg-card p-4 shadow ring-1 ring-foreground/5">
              <div class="mb-3 flex items-center gap-2">
                <div class="font-medium">Handoff</div>
              </div>
              <div class="grid grid-cols-2 gap-3 text-sm">
                <div>
                  <div class="text-muted-foreground">Contract</div>
                  <div class="font-medium">v0</div>
                </div>
                <div>
                  <div class="text-muted-foreground">Rules</div>
                  <div class="font-medium">{{ enabledInstructions.length }}</div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </TabsContent>
    </Tabs>
  </div>
</template>
