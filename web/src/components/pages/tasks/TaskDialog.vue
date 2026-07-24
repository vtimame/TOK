<script lang="ts" setup>
import type { Project } from "@/components/pages/projects";
import type { TaskDraft } from "@/api/queries/tasks.ts";
import FormController from "@/components/common/form/FormController.vue";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { required } from "@regle/rules";
import { useRegle } from "@regle/core";
import { VisuallyHidden } from "reka-ui";
import type { AcceptableValue } from "reka-ui";
import { reactive, watch } from "vue";

const emits = defineEmits<{
  "update:open": [value: boolean];
  save: [value: TaskDraft];
}>();

const props = defineProps<{
  open?: boolean;
  projects: Project[];
  projectId?: string;
  saving?: boolean;
}>();

const state = reactive({
  projectId: props.projectId || "",
  title: "",
  description: "",
  acceptanceCriteria: "",
  notes: "",
});

const { r$ } = useRegle(
  state,
  {
    projectId: { required },
    title: { required },
  },
  {},
);

watch(
  () => [props.open, props.projectId] as const,
  ([open, projectId]) => {
    if (open) {
      state.projectId = projectId || "";
      r$.$reset();
      return;
    }
    setTimeout(() => {
      state.projectId = projectId || "";
      state.title = "";
      state.description = "";
      state.acceptanceCriteria = "";
      state.notes = "";
      r$.$reset();
    }, 500);
  },
);

function updateProject(value: AcceptableValue) {
  state.projectId = value === null ? "" : String(value);
}

async function save() {
  const { valid } = await r$.$validate();
  if (!valid) return;

  const project = props.projects.find((item) => String(item.id) === r$.$value.projectId);
  if (!project) return;

  emits("save", {
    project: project.name,
    title: r$.$value.title.trim(),
    description: state.description.trim() || undefined,
    acceptance_criteria: state.acceptanceCriteria.trim() || undefined,
    notes: state.notes.trim() || undefined,
  });
}
</script>

<template>
  <Dialog :open="props.open" @update:open="emits('update:open', $event)">
    <DialogContent :show-close-button="false">
      <DialogHeader>
        <DialogTitle>New task</DialogTitle>
        <VisuallyHidden>
          <DialogDescription></DialogDescription>
        </VisuallyHidden>
      </DialogHeader>

      <div class="space-y-4">
        <FormController label="Project" required :errors="r$.projectId.$errors">
          <Select
            :model-value="state.projectId"
            :disabled="props.saving"
            @update:model-value="updateProject"
          >
            <SelectTrigger class="w-full">
              <SelectValue placeholder="Select project" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem v-for="project in props.projects" :key="project.id" :value="String(project.id)">
                {{ project.displayName }}
              </SelectItem>
            </SelectContent>
          </Select>
        </FormController>

        <FormController label="Task title" required :errors="r$.title.$errors">
          <Input v-model="r$.$value.title" :disabled="props.saving" />
        </FormController>

        <FormController label="Description">
          <Textarea v-model="state.description" :disabled="props.saving" />
        </FormController>

        <FormController label="Acceptance criteria">
          <Textarea v-model="state.acceptanceCriteria" :disabled="props.saving" />
        </FormController>

        <FormController label="Notes">
          <Textarea v-model="state.notes" :disabled="props.saving" />
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
