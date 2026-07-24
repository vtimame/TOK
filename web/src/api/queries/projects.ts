import type { CreateProjectInput } from "@/api/generated/models/CreateProjectInput.ts";
import type { ProjectListResponse } from "@/api/generated/models/ProjectListResponse.ts";
import { listProjectsQueryKey, useListProjects } from "@/api/generated/hooks/useListProjects.ts";
import { useCreateProject } from "@/api/generated/hooks/useCreateProject.ts";
import type { Project } from "@/components/pages/projects";
import { projectFromApi } from "@/api/mappers.ts";
import { useQueryClient } from "@tanstack/vue-query";

export type ProjectDraft = CreateProjectInput;

export const projectQueryKeys = {
  all: listProjectsQueryKey,
};

export function useProjectsQuery() {
  return useListProjects<Project[]>({
    query: {
      select: (response: ProjectListResponse) => (response.projects ?? []).map(projectFromApi),
    },
  });
}

export function useCreateProjectMutation() {
  const queryClient = useQueryClient();

  return useCreateProject({
    mutation: {
      onSuccess: () => queryClient.invalidateQueries({ queryKey: listProjectsQueryKey() }),
    },
  });
}
