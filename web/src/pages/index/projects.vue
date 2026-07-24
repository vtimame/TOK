<script lang="ts" setup>
import { Button } from "@/components/ui/button";
import { useTitle } from "@vueuse/core";
import {
  Table,
  TableBody,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
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
const mockProjectRows = Array.from({ length: 80 }, (_, index) => {
  const ready = index % 6;
  const blocked = index % 5 === 0 ? 1 : 0;
  const done = index * 2;

  return {
    id: index + 1,
    name: `mock-project-${String(index + 1).padStart(2, "0")}`,
    displayName: `Mock project ${String(index + 1).padStart(2, "0")}`,
    path: `/home/vtima/projects/mock-${String(index + 1).padStart(2, "0")}`,
    status: blocked ? "Blocked" : ready ? "Active" : "Idle",
    ready,
    blocked,
    done,
    total: ready + blocked + done + 3,
  };
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
      <div class="flex h-full min-h-0 flex-col overflow-hidden rounded-lg bg-card shadow ring-1 ring-foreground/5">
        <Table container-class="h-full min-h-0" class="min-w-[760px]">
          <TableHeader class="sticky top-0 z-10 bg-card shadow-[0_1px_0_hsl(var(--border))]">
            <TableRow>
              <TableHead class="w-16">ID</TableHead>
              <TableHead>Project</TableHead>
              <TableHead>Status</TableHead>
              <TableHead class="text-right">Ready</TableHead>
              <TableHead class="text-right">Blocked</TableHead>
              <TableHead class="text-right">Done</TableHead>
              <TableHead class="text-right">Total</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="project in mockProjectRows" :key="project.id">
              <TableCell class="text-muted-foreground">#{{ project.id }}</TableCell>
              <TableCell>
                <div class="font-medium">{{ project.displayName }}</div>
                <div class="max-w-[24rem] truncate text-sm text-muted-foreground">{{ project.path }}</div>
              </TableCell>
              <TableCell>
                <span class="rounded-full border px-2 py-0.5 text-xs text-muted-foreground">
                  {{ project.status }}
                </span>
              </TableCell>
              <TableCell class="text-right">{{ project.ready }}</TableCell>
              <TableCell class="text-right">{{ project.blocked }}</TableCell>
              <TableCell class="text-right">{{ project.done }}</TableCell>
              <TableCell class="text-right font-medium">{{ project.total }}</TableCell>
            </TableRow>
          </TableBody>
          <TableFooter class="sticky bottom-0 z-10 bg-muted shadow-[0_-1px_0_hsl(var(--border))]">
            <TableRow>
              <TableCell colspan="2">
                Showing {{ mockProjectRows.length }} mock projects
                <span v-if="projects.length" class="text-muted-foreground">
                  · {{ projects.length }} from API hidden for layout test
                </span>
              </TableCell>
              <TableCell class="text-right text-muted-foreground" colspan="5">Footer stays fixed while rows scroll</TableCell>
            </TableRow>
          </TableFooter>
        </Table>
      </div>
    </div>
  </div>
</template>
