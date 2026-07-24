<script lang="ts" setup>
import { Button } from "@/components/ui/button";
import { useTitle } from "@vueuse/core";
import EmptyProjectsList from "@/components/pages/projects/EmptyProjectsList.vue";
import ProjectsList from "@/components/pages/projects/ProjectsList.vue";
import { computed, ref } from "vue";
import ProjectDialog from "@/components/pages/projects/ProjectDialog.vue";
import {
  type ProjectDraft,
  useCreateProjectMutation,
  useProjectsQuery,
} from "@/api/queries/projects.ts";
import { toastApiError } from "@/api/axios.ts";
import { useRoute } from "vue-router";

const route = useRoute();
const showProjectDialog = ref(false);
const projectsQuery = useProjectsQuery();
const createProjectMutation = useCreateProjectMutation();
const projects = computed(() => projectsQuery.data.value ?? []);
const hasProjectRoute = computed(() => "project" in route.params);

async function saveProject(input: ProjectDraft) {
  try {
    await createProjectMutation.mutateAsync({ data: input });
    showProjectDialog.value = false;
  } catch (error) {
    toastApiError(error, {
      409: "Project already exists.",
    });
  }
}

useTitle("Projects");
</script>

<template>
  <RouterView v-if="hasProjectRoute" />

  <div v-else class="mx-auto flex h-svh w-full max-w-5xl flex-col gap-4 overflow-hidden px-4 py-18">
    <div class="flex shrink-0 items-center justify-between">
      <div class="text-2xl font-bold">Projects</div>
      <Button @click="showProjectDialog = true">New project</Button>
    </div>

    <ProjectDialog
      v-model:open="showProjectDialog"
      :saving="createProjectMutation.isPending.value"
      @save="saveProject"
    />

    <div class="min-h-0 flex-1">
      <div class="flex max-h-full flex-col bg-card rounded-lg shadow ring-1 ring-foreground/5">
        <div class="min-h-0 overflow-y-auto custom-scrollbar">
          <div v-if="projectsQuery.isPending.value" class="p-6 text-sm text-muted-foreground">
            Loading projects...
          </div>
          <div v-else-if="projectsQuery.isError.value" class="p-6 text-sm text-destructive">
            Failed to load projects.
          </div>
          <EmptyProjectsList v-else-if="projects.length === 0" @create="showProjectDialog = true" />
          <ProjectsList v-else :projects="projects" />
        </div>

        <div v-if="projects.length > 0" class="shrink-0 border-t">
          <div class="p-3">footer</div>
        </div>
      </div>
    </div>
  </div>
</template>
