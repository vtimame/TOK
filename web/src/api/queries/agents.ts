import type { CreateAgentInput } from "@/api/generated/models/CreateAgentInput.ts";
import type { AgentListResponse } from "@/api/generated/models/AgentListResponse.ts";
import { useCreateAgent } from "@/api/generated/hooks/useCreateAgent.ts";
import { useDeleteAgent } from "@/api/generated/hooks/useDeleteAgent.ts";
import { useUpdateAgent } from "@/api/generated/hooks/useUpdateAgent.ts";
import { listAgentsQueryKey, useListAgents } from "@/api/generated/hooks/useListAgents.ts";
import type { Agent } from "@/api/mappers.ts";
import { agentFromApi } from "@/api/mappers.ts";
import { useQueryClient } from "@tanstack/vue-query";

export type AgentDraft = CreateAgentInput;
export type AgentsPage = {
  agents: Agent[];
};

export const agentQueryKeys = {
  all: listAgentsQueryKey,
};

export function useAgentsQuery() {
  return useListAgents<AgentsPage>({
    query: {
      select: (response: AgentListResponse) => ({
        agents: (response.agents ?? []).map(agentFromApi),
      }),
    },
  });
}

export function useCreateAgentMutation() {
  const queryClient = useQueryClient();

  return useCreateAgent({
    mutation: {
      onSuccess: () => queryClient.invalidateQueries({ queryKey: listAgentsQueryKey() }),
    },
  });
}

export function useUpdateAgentMutation() {
  const queryClient = useQueryClient();

  return useUpdateAgent({
    mutation: {
      onSuccess: () => queryClient.invalidateQueries({ queryKey: listAgentsQueryKey() }),
    },
  });
}

export function useDeleteAgentMutation() {
  const queryClient = useQueryClient();

  return useDeleteAgent({
    mutation: {
      onSuccess: () => queryClient.invalidateQueries({ queryKey: listAgentsQueryKey() }),
    },
  });
}
