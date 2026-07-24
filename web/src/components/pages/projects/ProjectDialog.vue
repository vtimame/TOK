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

const emits = defineEmits<{ "update:open": [v: boolean] }>();
const props = defineProps<{
  open?: boolean;
  project?: Project;
}>();

const state = reactive<{ name: string }>({
  name: props.project?.name || "",
});

const { r$ } = useRegle(
  state,
  {
    name: { required },
  },
  {},
);

watch(
  () => props.open,
  (v: boolean) => {
    if (!v)
      setTimeout(() => {
        state.name = "";
        r$.$reset();
      }, 500);
  },
);
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

        <div class="flex items-center justify-end gap-x-2">
          <Button variant="secondary" @click="emits('update:open', false)">Cancel</Button>
          <Button :disabled="r$.$invalid">Save</Button>
        </div>
      </div>
    </DialogContent>
  </Dialog>
</template>
