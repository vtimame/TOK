<script lang="ts" setup>
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Copy } from "@lucide/vue";
import { toast } from "vue-sonner";

const emits = defineEmits<{
  "update:open": [value: boolean];
}>();
const props = defineProps<{
  open?: boolean;
  agentName?: string;
  token?: string;
}>();

async function copyToken() {
  if (!props.token) return;
  await navigator.clipboard.writeText(props.token);
  toast("Agent token copied");
}
</script>

<template>
  <Dialog :open="props.open" @update:open="emits('update:open', $event)">
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Agent token</DialogTitle>
        <DialogDescription>
          Copy this token now. It is shown only once.
        </DialogDescription>
      </DialogHeader>

      <div class="space-y-4">
        <div>
          <div class="text-sm font-medium">{{ props.agentName }}</div>
          <div class="mt-2 rounded-md border bg-muted p-3 font-mono text-xs break-all">
            {{ props.token }}
          </div>
        </div>

        <div class="flex items-center justify-end gap-x-2">
          <Button variant="secondary" @click="emits('update:open', false)">Close</Button>
          <Button @click="copyToken">
            <Copy />
            Copy token
          </Button>
        </div>
      </div>
    </DialogContent>
  </Dialog>
</template>
