<script lang="ts" setup="">
import type { Project } from "@/components/pages/projects/index.ts";
import ProjectRow from "@/components/pages/projects/ProjectRow.vue";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
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
import { useUpdateProjectIndex } from "@/api/generated/hooks/useUpdateProjectIndex.ts";
import { toastApiError } from "@/api/axios.ts";
import { toast } from "vue-sonner";
import { ref } from "vue";
import { useDeleteProjectMutation } from "@/api/queries/projects.ts";

const props = defineProps<{
  projects: Project[];
  loading?: boolean;
  error?: boolean;
}>();

const emits = defineEmits<{
  create: [];
  edit: [project: Project];
}>();

const projectToDelete = ref<Project>();
const updateIndexMutation = useUpdateProjectIndex({
  mutation: {
    onSuccess: (response) => {
      toast("Index updated", {
        description: `${response.project_name}: ${response.indexed_documents} documents indexed.`,
      });
    },
    onError: (error) => {
      toastApiError(error);
    },
  },
});
const deleteProjectMutation = useDeleteProjectMutation();

async function deleteProject() {
  if (!projectToDelete.value) return;
  try {
    const project = projectToDelete.value;
    await deleteProjectMutation.mutateAsync({ project: project.name });
    toast("Project deleted", {
      description: project.displayName,
    });
    projectToDelete.value = undefined;
  } catch (error) {
    toastApiError(error);
  }
}

function updateDeleteDialogOpen(open: boolean) {
  if (!open) {
    projectToDelete.value = undefined;
  }
}
</script>

<template>
  <AlertDialog :open="Boolean(projectToDelete)" @update:open="updateDeleteDialogOpen">
    <AlertDialogContent>
      <AlertDialogHeader>
        <AlertDialogTitle>Delete project?</AlertDialogTitle>
        <AlertDialogDescription>
          This will delete {{ projectToDelete?.displayName || "this project" }} and all of its tasks.
        </AlertDialogDescription>
      </AlertDialogHeader>
      <AlertDialogFooter>
        <AlertDialogCancel :disabled="deleteProjectMutation.isPending.value">Cancel</AlertDialogCancel>
        <AlertDialogAction
          class="bg-destructive text-destructive-foreground hover:bg-destructive/90"
          :disabled="deleteProjectMutation.isPending.value"
          @click="deleteProject"
        >
          Delete project
        </AlertDialogAction>
      </AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>

  <Table container-class="max-h-full min-h-0 custom-scrollbar">
    <TableHeader class="sticky top-0 z-10 bg-card shadow-[0_1px_0_hsl(var(--border))]">
      <TableRow>
        <TableHead class="w-16 pl-4">ID</TableHead>
        <TableHead>Project</TableHead>
        <TableHead>Path</TableHead>
        <TableHead>Status</TableHead>
        <TableHead class="text-right">Ready</TableHead>
        <TableHead class="text-right">Blocked</TableHead>
        <TableHead class="text-right">Done</TableHead>
        <TableHead class="text-right">Total</TableHead>
        <TableHead class="text-right pr-4"></TableHead>
      </TableRow>
    </TableHeader>
    <TableBody>
      <TableRow v-if="props.error">
        <TableCell colspan="9" class="h-24 text-center text-destructive">
          Failed to load projects.
        </TableCell>
      </TableRow>
      <TableRow v-if="!props.loading && props.projects.length === 0">
        <TableCell colspan="9" class="h-36 text-center">
          <div class="flex flex-col items-center gap-3">
            <div>
              <div class="font-medium">No projects yet</div>
              <div class="text-sm text-muted-foreground">
                Create a project to start tracking tasks.
              </div>
            </div>
            <Button size="sm" @click="emits('create')">Create project</Button>
          </div>
        </TableCell>
      </TableRow>
      <ProjectRow
        v-for="project in props.projects"
        :key="project.id"
        :project="project"
        :updating-index="updateIndexMutation.isPending.value"
        @edit="emits('edit', project)"
        @delete="projectToDelete = project"
        @update-index="updateIndexMutation.mutate({ project: project.name })"
      />
    </TableBody>
  </Table>
</template>
