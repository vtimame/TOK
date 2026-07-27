<script lang="ts" setup>
import { Button } from "@/components/ui/button";
import { useTitle } from "@vueuse/core";
import { computed, ref } from "vue";
import ProjectDialog from "@/components/pages/projects/ProjectDialog.vue";
import {
  type ProjectDraft,
  useCreateProjectMutation,
  useUpdateProjectMutation,
  useInfiniteProjectsQuery,
} from "@/api/queries/projects.ts";
import { toastApiError } from "@/api/axios.ts";
import { useRoute } from "vue-router";
import ProjectsTable from "@/components/pages/projects/ProjectsTable.vue";
import type { Project } from "@/components/pages/projects";

const route = useRoute();
const showProjectDialog = ref(false);
const editingProject = ref<Project>();
const pageSize = ref(25);
const projectListParams = computed(() => ({
  limit: String(pageSize.value),
}));
const projectsQuery = useInfiniteProjectsQuery(projectListParams);
const createProjectMutation = useCreateProjectMutation();
const updateProjectMutation = useUpdateProjectMutation();
const projectPages = computed(() => projectsQuery.data.value?.pages ?? []);
const projects = computed(() => projectPages.value.flatMap((page) => page.projects));
const totalProjects = computed(() => projectPages.value[projectPages.value.length - 1]?.total ?? 0);
const hasProjectRoute = computed(() => "project" in route.params);

async function saveProject(input: ProjectDraft) {
  try {
    if (editingProject.value) {
      await updateProjectMutation.mutateAsync({
        project: editingProject.value.name,
        data: {
          name: input.name,
          display_name: input.display_name || input.name,
          path: input.path,
        },
      });
    } else {
      await createProjectMutation.mutateAsync({ data: input });
    }
    editingProject.value = undefined;
    showProjectDialog.value = false;
  } catch (error) {
    toastApiError(error, {
      409: "Project already exists.",
    });
  }
}

function createProject() {
  editingProject.value = undefined;
  showProjectDialog.value = true;
}

function editProject(project: Project) {
  editingProject.value = project;
  showProjectDialog.value = true;
}

function updateProjectDialogOpen(open: boolean) {
  showProjectDialog.value = open;
  if (!open) {
    editingProject.value = undefined;
  }
}

useTitle("Projects");
</script>

<template>
  <RouterView v-if="hasProjectRoute" />

  <div v-else class="mx-auto flex h-svh w-full max-w-6xl flex-col gap-4 overflow-hidden px-4 py-18">
    <div class="flex shrink-0 items-center justify-between">
      <div class="text-2xl font-bold">Projects</div>
      <Button @click="createProject" size="sm">New project</Button>
    </div>

    <ProjectDialog
      v-model:open="showProjectDialog"
      :project="editingProject"
      :saving="createProjectMutation.isPending.value || updateProjectMutation.isPending.value"
      @save="saveProject"
      @update:open="updateProjectDialogOpen"
    />

    <div class="min-h-0 flex-1 overflow-hidden">
      <div
        class="flex max-h-full min-h-0 flex-col overflow-hidden rounded-lg bg-card shadow ring-1 ring-foreground/5"
      >
        <ProjectsTable
          :projects="projects"
          :total="totalProjects"
          :page-size="pageSize"
          :loading="projectsQuery.isPending.value"
          :fetching-more="projectsQuery.isFetchingNextPage.value"
          :has-more="projectsQuery.hasNextPage.value"
          :error="projectsQuery.isError.value"
          @create="createProject"
          @edit="editProject"
          @load-more="projectsQuery.fetchNextPage()"
          @update:page-size="pageSize = $event"
        />
      </div>
    </div>
  </div>
</template>
