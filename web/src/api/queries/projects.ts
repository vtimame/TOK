import type { CreateProjectInput } from "@/api/generated/models/CreateProjectInput.ts";
import type { ListProjectsQueryParams } from "@/api/generated/models/ListProjects.ts";
import type { ProjectListResponse } from "@/api/generated/models/ProjectListResponse.ts";
import { useDeleteProject } from "@/api/generated/hooks/useDeleteProject.ts";
import { listProjectsQueryKey, useListProjects } from "@/api/generated/hooks/useListProjects.ts";
import { useCreateProject } from "@/api/generated/hooks/useCreateProject.ts";
import { useUpdateProject } from "@/api/generated/hooks/useUpdateProject.ts";
import type { Project } from "@/components/pages/projects";
import { projectFromApi } from "@/api/mappers.ts";
import { useQueryClient } from "@tanstack/vue-query";
import type { MaybeRefOrGetter } from "vue";

export type ProjectDraft = CreateProjectInput;
export type ProjectsPage = {
  projects: Project[];
  total: number;
  limit: number;
  offset: number;
};

export const projectQueryKeys = {
  all: listProjectsQueryKey,
};

export function useProjectsQuery(params?: MaybeRefOrGetter<ListProjectsQueryParams>) {
  return useListProjects<ProjectsPage>(
    { params },
    {
      query: {
        select: (response: ProjectListResponse) => ({
          projects: (response.projects ?? []).map(projectFromApi),
          total: response.total,
          limit: response.limit,
          offset: response.offset,
        }),
      },
    },
  );
}

export function useCreateProjectMutation() {
  const queryClient = useQueryClient();

  return useCreateProject({
    mutation: {
      onSuccess: () => queryClient.invalidateQueries({ queryKey: listProjectsQueryKey() }),
    },
  });
}

export function useUpdateProjectMutation() {
  const queryClient = useQueryClient();

  return useUpdateProject({
    mutation: {
      onSuccess: () => queryClient.invalidateQueries({ queryKey: listProjectsQueryKey() }),
    },
  });
}

export function useDeleteProjectMutation() {
  const queryClient = useQueryClient();

  return useDeleteProject({
    mutation: {
      onSuccess: () => queryClient.invalidateQueries({ queryKey: listProjectsQueryKey() }),
    },
  });
}
