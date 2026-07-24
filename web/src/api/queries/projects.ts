import { createProject } from "@/api/generated/client/createProject.ts";
import { listProjects } from "@/api/generated/client/listProjects.ts";
import type { CreateProjectInput } from "@/api/generated/models/CreateProjectInput.ts";
import type { ProjectOutput } from "@/api/generated/models/ProjectOutput.ts";
import type { Project } from "@/components/pages/projects";
import { useMutation, useQuery, useQueryClient } from "@tanstack/vue-query";

export type ProjectDraft = CreateProjectInput;

export const projectQueryKeys = {
  all: ["projects"] as const,
};

export function useProjectsQuery() {
  return useQuery({
    queryKey: projectQueryKeys.all,
    queryFn: async () => {
      const response = await listProjects();
      return (response.projects ?? []).map(projectFromApi);
    },
  });
}

export function useCreateProjectMutation() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (input: ProjectDraft) => createProject({ data: input }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: projectQueryKeys.all }),
  });
}

function projectFromApi(project: ProjectOutput): Project {
  return {
    id: project.id,
    name: project.name,
    displayName: project.display_name,
    path: project.path,
    createdAt: project.created_at,
    updatedAt: project.updated_at,
    tasksCount: project.tasks_count,
    taskCounts: project.task_counts,
    agents: (project.agents ?? []).map((actor) => agentIconValue(actor.name)),
  };
}

function agentIconValue(name: string): string {
  const normalized = name.trim().toLowerCase();
  for (const known of ["codex", "openai", "claude", "gemini", "cursor", "copilot", "grok"]) {
    if (normalized.includes(known)) {
      return known;
    }
  }
  return normalized || "agent";
}
