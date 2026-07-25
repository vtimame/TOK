import type { ListTasksQueryParams } from "@/api/generated/models/ListTasks.ts";
import type { TaskListResponse } from "@/api/generated/models/TaskListResponse.ts";
import type { TaskShowResponse } from "@/api/generated/models/TaskShowResponse.ts";
import { useBlockTask } from "@/api/generated/hooks/useBlockTask.ts";
import { useClaimTask } from "@/api/generated/hooks/useClaimTask.ts";
import { useCommentTask } from "@/api/generated/hooks/useCommentTask.ts";
import { useCompleteTask } from "@/api/generated/hooks/useCompleteTask.ts";
import { listTasksQueryKey, useListTasks } from "@/api/generated/hooks/useListTasks.ts";
import { showTaskQueryKey, useShowTask } from "@/api/generated/hooks/useShowTask.ts";
import { useProgressTask } from "@/api/generated/hooks/useProgressTask.ts";
import { useUnblockTask } from "@/api/generated/hooks/useUnblockTask.ts";
import { useCreateTask } from "@/api/generated/hooks/useCreateTask.ts";
import type { CreateTaskInput } from "@/api/generated/models/CreateTaskInput.ts";
import type { Task, TaskDependency, TaskEvent } from "@/api/mappers.ts";
import { taskDependencyFromApi, taskEventFromApi, taskFromApi } from "@/api/mappers.ts";
import { listProjectsQueryKey } from "@/api/generated/hooks/useListProjects.ts";
import { useQueryClient } from "@tanstack/vue-query";
import type { QueryClient } from "@tanstack/vue-query";
import type { MaybeRefOrGetter } from "vue";
import { projectQueryKeys } from "@/api/queries/projects.ts";

export type TaskDraft = CreateTaskInput & {
  project: string;
};

export type TasksPage = {
  tasks: Task[];
  total: number;
  limit: number;
  offset: number;
};

export type TaskDetails = {
  task: Task;
  events: TaskEvent[];
  dependencies: TaskDependency[];
};

export const taskQueryKeys = {
  all: listTasksQueryKey,
  details: showTaskQueryKey,
};

export function useTasksQuery(params?: MaybeRefOrGetter<ListTasksQueryParams>) {
  return useListTasks<TasksPage>(
    { params },
    {
      query: {
        select: (response: TaskListResponse) => ({
          tasks: (response.tasks ?? []).map(taskFromApi),
          total: response.total,
          limit: response.limit,
          offset: response.offset,
        }),
      },
    },
  );
}

export function useTaskQuery(id: MaybeRefOrGetter<string | undefined>) {
  return useShowTask<TaskDetails>(
    { id },
    {
      query: {
        select: (response: TaskShowResponse) => ({
          task: taskFromApi(response.task),
          events: (response.events ?? []).map(taskEventFromApi),
          dependencies: (response.dependencies ?? []).map(taskDependencyFromApi),
        }),
      },
    },
  );
}

export function useCreateTaskMutation() {
  const queryClient = useQueryClient();

  return useCreateTask({
    mutation: {
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: listTasksQueryKey() });
        queryClient.invalidateQueries({ queryKey: listProjectsQueryKey() });
      },
    },
  });
}

export function useClaimTaskMutation() {
  const queryClient = useQueryClient();

  return useClaimTask({
    mutation: {
      onSuccess: () => invalidateTaskQueries(queryClient),
    },
  });
}

export function useCompleteTaskMutation() {
  const queryClient = useQueryClient();

  return useCompleteTask({
    mutation: {
      onSuccess: () => invalidateTaskQueries(queryClient),
    },
  });
}

export function useCommentTaskMutation() {
  const queryClient = useQueryClient();

  return useCommentTask({
    mutation: {
      onSuccess: () => invalidateTaskQueries(queryClient),
    },
  });
}

export function useProgressTaskMutation() {
  const queryClient = useQueryClient();

  return useProgressTask({
    mutation: {
      onSuccess: () => invalidateTaskQueries(queryClient),
    },
  });
}

export function useBlockTaskMutation() {
  const queryClient = useQueryClient();

  return useBlockTask({
    mutation: {
      onSuccess: () => invalidateTaskQueries(queryClient),
    },
  });
}

export function useUnblockTaskMutation() {
  const queryClient = useQueryClient();

  return useUnblockTask({
    mutation: {
      onSuccess: () => invalidateTaskQueries(queryClient),
    },
  });
}

function invalidateTaskQueries(queryClient: QueryClient) {
  queryClient.invalidateQueries({ queryKey: listTasksQueryKey() });
  queryClient.invalidateQueries({
    predicate: (query) => {
      const key = query.queryKey[0] as { url?: string } | undefined;
      return key?.url === projectQueryKeys.tasks(undefined)[0].url;
    },
  });
  queryClient.invalidateQueries({
    predicate: (query) => {
      const key = query.queryKey[0] as { url?: string } | undefined;
      return key?.url === "/api/tasks/:id";
    },
  });
  queryClient.invalidateQueries({ queryKey: listProjectsQueryKey() });
}
