import type { ListTasksQueryParams } from "@/api/generated/models/ListTasks.ts";
import type { TaskListResponse } from "@/api/generated/models/TaskListResponse.ts";
import { listTasksQueryKey, useListTasks } from "@/api/generated/hooks/useListTasks.ts";
import type { Task } from "@/api/mappers.ts";
import { taskFromApi } from "@/api/mappers.ts";
import type { MaybeRefOrGetter } from "vue";

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
