import type { CreateProjectInput } from "@/api/generated/models/CreateProjectInput.ts";
import type { ListProjectsQueryParams } from "@/api/generated/models/ListProjects.ts";
import type { ProjectInstructionInput } from "@/api/generated/models/ProjectInstructionInput.ts";
import type { ProjectInstructionListResponse } from "@/api/generated/models/ProjectInstructionListResponse.ts";
import type { ProjectInstructionOutput } from "@/api/generated/models/ProjectInstructionOutput.ts";
import type { ProjectListResponse } from "@/api/generated/models/ProjectListResponse.ts";
import {
  listProjectInstructionsQueryKey,
  useListProjectInstructions,
} from "@/api/generated/hooks/useListProjectInstructions.ts";
import { useCreateProjectInstruction } from "@/api/generated/hooks/useCreateProjectInstruction.ts";
import { useDeleteProjectInstruction } from "@/api/generated/hooks/useDeleteProjectInstruction.ts";
import { useDisableProjectInstruction } from "@/api/generated/hooks/useDisableProjectInstruction.ts";
import { useEnableProjectInstruction } from "@/api/generated/hooks/useEnableProjectInstruction.ts";
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
export type ProjectInstruction = {
  id: number;
  projectId: number;
  scope: string;
  title: string;
  body: string;
  priority: string;
  enabled: boolean;
  source: string;
  createdAt: string;
  updatedAt: string;
};
export type ProjectInstructionDraft = ProjectInstructionInput;

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

export function useProjectInstructionsQuery(project: MaybeRefOrGetter<string | undefined>) {
  return useListProjectInstructions<ProjectInstruction[]>(
    {
      project,
      params: {
        includeDisabled: "true",
      },
    },
    {
      query: {
        select: (response: ProjectInstructionListResponse) =>
          (response.instructions ?? []).map(projectInstructionFromApi),
      },
    },
  );
}

export function useCreateProjectInstructionMutation() {
  const queryClient = useQueryClient();

  return useCreateProjectInstruction({
    mutation: {
      onSuccess: () => invalidateProjectInstructionQueries(queryClient),
    },
  });
}

export function useEnableProjectInstructionMutation() {
  const queryClient = useQueryClient();

  return useEnableProjectInstruction({
    mutation: {
      onSuccess: () => invalidateProjectInstructionQueries(queryClient),
    },
  });
}

export function useDisableProjectInstructionMutation() {
  const queryClient = useQueryClient();

  return useDisableProjectInstruction({
    mutation: {
      onSuccess: () => invalidateProjectInstructionQueries(queryClient),
    },
  });
}

export function useDeleteProjectInstructionMutation() {
  const queryClient = useQueryClient();

  return useDeleteProjectInstruction({
    mutation: {
      onSuccess: () => invalidateProjectInstructionQueries(queryClient),
    },
  });
}

function projectInstructionFromApi(instruction: ProjectInstructionOutput): ProjectInstruction {
  return {
    id: instruction.id,
    projectId: instruction.project_id,
    scope: instruction.scope,
    title: instruction.title,
    body: instruction.body,
    priority: instruction.priority,
    enabled: instruction.enabled,
    source: instruction.source,
    createdAt: instruction.created_at,
    updatedAt: instruction.updated_at,
  };
}

function invalidateProjectInstructionQueries(queryClient: ReturnType<typeof useQueryClient>) {
  queryClient.invalidateQueries({
    predicate: (query) => {
      const key = query.queryKey[0] as { url?: string } | undefined;
      return key?.url === "/api/projects/:project/instructions";
    },
  });
}
