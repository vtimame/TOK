<script lang="ts" setup>
import type { Agent } from "@/components/pages/agents";
import AgentRow from "@/components/pages/agents/AgentRow.vue";
import { toastApiError } from "@/api/axios.ts";
import { useDeleteAgentMutation } from "@/api/queries/agents.ts";
import { Button } from "@/components/ui/button";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { ref } from "vue";
import { toast } from "vue-sonner";

const props = defineProps<{
  agents: Agent[];
  loading?: boolean;
  error?: boolean;
}>();

const emits = defineEmits<{
  create: [];
  edit: [agent: Agent];
}>();

const agentToDelete = ref<Agent>();
const deleteAgentMutation = useDeleteAgentMutation();

async function deleteAgent() {
  if (!agentToDelete.value) return;
  try {
    await deleteAgentMutation.mutateAsync({ id: String(agentToDelete.value.id) });
    toast("Agent deleted", {
      description: agentToDelete.value.name,
    });
    agentToDelete.value = undefined;
  } catch (error) {
    toastApiError(error);
  }
}

function updateDeleteDialogOpen(open: boolean) {
  if (!open) {
    agentToDelete.value = undefined;
  }
}
</script>

<template>
  <AlertDialog :open="Boolean(agentToDelete)" @update:open="updateDeleteDialogOpen">
    <AlertDialogContent>
      <AlertDialogHeader>
        <AlertDialogTitle>Delete agent?</AlertDialogTitle>
        <AlertDialogDescription>
          This will delete {{ agentToDelete?.name || "this agent" }} registration.
        </AlertDialogDescription>
      </AlertDialogHeader>
      <AlertDialogFooter>
        <AlertDialogCancel :disabled="deleteAgentMutation.isPending.value"
          >Cancel</AlertDialogCancel
        >
        <AlertDialogAction
          class="bg-destructive text-destructive-foreground hover:bg-destructive/90"
          :disabled="deleteAgentMutation.isPending.value"
          @click="deleteAgent"
        >
          Delete agent
        </AlertDialogAction>
      </AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>

  <Table container-class="max-h-full min-h-0 custom-scrollbar">
    <TableHeader class="sticky top-0 z-10 bg-card shadow-[0_1px_0_hsl(var(--border))]">
      <TableRow>
        <TableHead class="w-16 pl-4">ID</TableHead>
        <TableHead>Agent</TableHead>
        <TableHead class="text-right">Tasks</TableHead>
        <TableHead class="text-right">Events</TableHead>
        <TableHead class="text-right">Last activity</TableHead>
        <TableHead class="text-right pr-4"></TableHead>
      </TableRow>
    </TableHeader>
    <TableBody>
      <TableRow v-if="props.error">
        <TableCell colspan="6" class="h-24 text-center text-destructive">
          Failed to load agents.
        </TableCell>
      </TableRow>
      <TableRow v-if="!props.loading && props.agents.length === 0">
        <TableCell colspan="6" class="h-36 text-center">
          <div class="flex flex-col items-center gap-3">
            <div>
              <div class="font-medium">No agents yet</div>
              <div class="text-sm text-muted-foreground">
                Create an agent to issue an API token.
              </div>
            </div>
            <Button size="sm" @click="emits('create')">Create agent</Button>
          </div>
        </TableCell>
      </TableRow>
      <AgentRow
        v-for="agent in props.agents"
        :key="agent.id"
        :agent="agent"
        @edit="emits('edit', agent)"
        @delete="agentToDelete = agent"
      />
    </TableBody>
  </Table>
</template>
