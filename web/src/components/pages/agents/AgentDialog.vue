<script lang="ts" setup>
import type { Agent } from "@/components/pages/agents";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import FormController from "@/components/common/form/FormController.vue";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { VisuallyHidden } from "reka-ui";
import { reactive, watch } from "vue";
import { useRegle } from "@regle/core";
import { required } from "@regle/rules";
import type { AgentDraft } from "@/api/queries/agents.ts";

const emits = defineEmits<{
  "update:open": [value: boolean];
  save: [value: AgentDraft];
}>();
const props = defineProps<{
  open?: boolean;
  agent?: Agent;
  saving?: boolean;
}>();

const state = reactive<{ name: string }>({
  name: props.agent?.name || "",
});

const { r$ } = useRegle(
  state,
  {
    name: { required },
  },
  {},
);

watch(
  () => [props.open, props.agent] as const,
  ([open, agent]) => {
    if (open) {
      state.name = agent?.name || "";
      r$.$reset();
      return;
    }
    setTimeout(() => {
      state.name = "";
      r$.$reset();
    }, 500);
  },
);

async function save() {
  const { valid } = await r$.$validate();
  if (!valid) return;
  emits("save", {
    name: r$.$value.name.trim(),
  });
}
</script>

<template>
  <Dialog :open="props.open" @update:open="emits('update:open', $event)">
    <DialogContent :show-close-button="false">
      <DialogHeader>
        <DialogTitle>{{ props.agent ? "Edit" : "New" }} agent</DialogTitle>
        <VisuallyHidden>
          <DialogDescription></DialogDescription>
        </VisuallyHidden>
      </DialogHeader>

      <div class="space-y-4">
        <FormController label="Agent name" required :errors="r$.name.$errors">
          <Input v-model="r$.$value.name" />
        </FormController>

        <div class="flex items-center justify-end gap-x-2">
          <Button variant="secondary" :disabled="props.saving" @click="emits('update:open', false)">
            Cancel
          </Button>
          <Button :disabled="r$.$invalid || props.saving" @click="save">
            {{ props.saving ? "Saving..." : "Save" }}
          </Button>
        </div>
      </div>
    </DialogContent>
  </Dialog>
</template>
