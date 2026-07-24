<script lang="ts" setup="">
import type { Project } from "@/components/pages/projects/index.ts";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { VisuallyHidden } from "reka-ui";
import FormController from "@/components/common/form/FormController.vue";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { reactive, watch } from "vue";
import { useRegle } from "@regle/core";
import { required } from "@regle/rules";
import type { ProjectDraft } from "@/api/queries/projects.ts";

const emits = defineEmits<{
  "update:open": [v: boolean];
  save: [v: ProjectDraft];
}>();
const props = defineProps<{
  open?: boolean;
  project?: Project;
  saving?: boolean;
}>();

const state = reactive<{ name: string; path: string }>({
  name: props.project?.name || "",
  path: props.project?.path || "",
});

const { r$ } = useRegle(
  state,
  {
    name: { required },
    path: { required },
  },
  {},
);

watch(
  () => [props.open, props.project] as const,
  ([open, project]) => {
    if (open) {
      state.name = project?.name || "";
      state.path = project?.path || "";
      r$.$reset();
      return;
    }
    if (!open)
      setTimeout(() => {
        state.name = "";
        state.path = "";
        r$.$reset();
      }, 500);
  },
);

async function save() {
  const { valid } = await r$.$validate();
  if (!valid) {
    return;
  }
  emits("save", {
    name: r$.$value.name.trim(),
    path: r$.$value.path.trim(),
  });
}
</script>

<template>
  <Dialog :open="props.open" @update:open="emits('update:open', $event)">
    <DialogContent :show-close-button="false">
      <DialogHeader>
        <DialogTitle>{{ props.project ? "Edit" : "New" }} project</DialogTitle>
        <VisuallyHidden>
          <DialogDescription></DialogDescription>
        </VisuallyHidden>
      </DialogHeader>

      <div class="space-y-4">
        <FormController label="Project name" required :errors="r$.name.$errors">
          <Input v-model="r$.$value.name" />
        </FormController>

        <FormController label="Project path" required :errors="r$.path.$errors">
          <Input v-model="r$.$value.path" />
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
