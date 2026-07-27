<script lang="ts" setup>
import { toastApiError } from "@/api/axios.ts";
import {
  type AgentDraft,
  useAgentsQuery,
  useCreateAgentMutation,
  useUpdateAgentMutation,
} from "@/api/queries/agents.ts";
import { agentFromApi } from "@/api/mappers.ts";
import AgentDialog from "@/components/pages/agents/AgentDialog.vue";
import AgentTokenDialog from "@/components/pages/agents/AgentTokenDialog.vue";
import AgentsTable from "@/components/pages/agents/AgentsTable.vue";
import { Button } from "@/components/ui/button";
import { useTitle } from "@vueuse/core";
import { computed, ref } from "vue";
import type { Agent } from "@/components/pages/agents";

const agentsQuery = useAgentsQuery();
const createAgentMutation = useCreateAgentMutation();
const updateAgentMutation = useUpdateAgentMutation();
const showAgentDialog = ref(false);
const editingAgent = ref<Agent>();
const createdToken = ref<{ agentName: string; token: string }>();
const agentsPage = computed(
  () =>
    agentsQuery.data.value ?? {
      agents: [],
    },
);
const agents = computed(() => agentsPage.value.agents);

function createAgent() {
  editingAgent.value = undefined;
  showAgentDialog.value = true;
}

function editAgent(agent: Agent) {
  editingAgent.value = agent;
  showAgentDialog.value = true;
}

async function saveAgent(input: AgentDraft) {
  try {
    if (editingAgent.value) {
      await updateAgentMutation.mutateAsync({
        id: String(editingAgent.value.id),
        data: input,
      });
    } else {
      const response = await createAgentMutation.mutateAsync({ data: input });
      const agent = agentFromApi(response.agent);
      createdToken.value = {
        agentName: agent.name,
        token: response.token,
      };
    }
    editingAgent.value = undefined;
    showAgentDialog.value = false;
  } catch (error) {
    toastApiError(error, {
      409: "Agent already exists.",
    });
  }
}

function updateAgentDialogOpen(open: boolean) {
  showAgentDialog.value = open;
  if (!open) {
    editingAgent.value = undefined;
  }
}

function updateTokenDialogOpen(open: boolean) {
  if (!open) {
    createdToken.value = undefined;
  }
}

useTitle("Agents");
</script>

<template>
  <div class="mx-auto flex h-svh w-full max-w-6xl flex-col gap-4 overflow-hidden px-4 py-18">
    <div class="flex shrink-0 items-center justify-between">
      <div class="text-2xl font-bold">Agents</div>
      <div class="flex items-center gap-x-3">
        <!--        <div class="text-sm text-muted-foreground">{{ agents.length }} registered</div>-->
        <Button @click="createAgent" size="sm">New agent</Button>
      </div>
    </div>

    <AgentDialog
      v-model:open="showAgentDialog"
      :agent="editingAgent"
      :saving="createAgentMutation.isPending.value || updateAgentMutation.isPending.value"
      @save="saveAgent"
      @update:open="updateAgentDialogOpen"
    />

    <AgentTokenDialog
      :open="Boolean(createdToken)"
      :agent-name="createdToken?.agentName"
      :token="createdToken?.token"
      @update:open="updateTokenDialogOpen"
    />

    <div class="min-h-0 flex-1 overflow-hidden">
      <div
        class="flex max-h-full min-h-0 flex-col overflow-hidden rounded-lg bg-card shadow ring-1 ring-foreground/5"
      >
        <AgentsTable
          :agents="agents"
          :loading="agentsQuery.isPending.value"
          :error="agentsQuery.isError.value"
          @create="createAgent"
          @edit="editAgent"
        />
      </div>
    </div>
  </div>
</template>
