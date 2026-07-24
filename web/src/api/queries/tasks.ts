import type { ListTasksQueryParams } from "@/api/generated/models/ListTasks.ts";
import type { TaskListResponse } from "@/api/generated/models/TaskListResponse.ts";
import { listTasksQueryKey, useListTasks } from "@/api/generated/hooks/useListTasks.ts";
import { useCreateTask } from "@/api/generated/hooks/useCreateTask.ts";
import type { CreateTaskInput } from "@/api/generated/models/CreateTaskInput.ts";
import type { Task } from "@/api/mappers.ts";
import { taskFromApi } from "@/api/mappers.ts";
import { listProjectsQueryKey } from "@/api/generated/hooks/useListProjects.ts";
import { useQueryClient } from "@tanstack/vue-query";
import type { MaybeRefOrGetter } from "vue";

export type TaskDraft = CreateTaskInput & {
  project: string;
};

export type TasksPage = {
  tasks: Task[];
  total: number;
  limit: number;
  offset: number;
};

export const taskQueryKeys = {
  all: listTasksQueryKey,
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
