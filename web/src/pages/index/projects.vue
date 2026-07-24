<script lang="ts" setup>
import { Button } from "@/components/ui/button";
import { useTitle } from "@vueuse/core";
import { computed, ref, watch } from "vue";
import ProjectDialog from "@/components/pages/projects/ProjectDialog.vue";
import {
  type ProjectDraft,
  useCreateProjectMutation,
  useProjectsQuery,
} from "@/api/queries/projects.ts";
import { toastApiError } from "@/api/axios.ts";
import { useRoute } from "vue-router";
import ProjectsTable from "@/components/pages/projects/ProjectsTable.vue";

const route = useRoute();
const showProjectDialog = ref(false);
const pageSize = ref(25);
const page = ref(1);
const offset = computed(() => (page.value - 1) * pageSize.value);
const projectListParams = computed(() => ({
  limit: String(pageSize.value),
  offset: String(offset.value),
}));
const projectsQuery = useProjectsQuery(projectListParams);
const createProjectMutation = useCreateProjectMutation();
const projectsPage = computed(
  () =>
    projectsQuery.data.value ?? {
      projects: [],
      total: 0,
      limit: pageSize.value,
      offset: offset.value,
    },
);
const projects = computed(() => projectsPage.value.projects);
const pageCount = computed(() => Math.max(1, Math.ceil(projectsPage.value.total / pageSize.value)));
const hasProjectRoute = computed(() => "project" in route.params);

watch(pageSize, () => {
  page.value = 1;
});

watch(pageCount, (nextPageCount) => {
  if (page.value > nextPageCount) {
    page.value = nextPageCount;
  }
});

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

    <div class="min-h-0 flex-1 overflow-hidden">
      <div
        class="flex max-h-full min-h-0 flex-col overflow-hidden rounded-lg bg-card shadow ring-1 ring-foreground/5"
      >
        <ProjectsTable
          :projects="projects"
          :total="projectsPage.total"
          :limit="projectsPage.limit"
          :offset="projectsPage.offset"
          :page="page"
          :page-count="pageCount"
          :page-size="pageSize"
          :loading="projectsQuery.isPending.value"
          :error="projectsQuery.isError.value"
          @create="showProjectDialog = true"
          @first-page="page = 1"
          @previous-page="page = Math.max(1, page - 1)"
          @next-page="page = Math.min(pageCount, page + 1)"
          @last-page="page = pageCount"
          @update:page-size="pageSize = $event"
        />
      </div>
    </div>
  </div>
</template>
