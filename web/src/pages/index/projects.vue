<script lang="ts" setup>
import { Button } from "@/components/ui/button";
import { useTitle } from "@vueuse/core";
import EmptyProjectsList from "@/components/pages/projects/EmptyProjectsList.vue";
import type { Project } from "@/components/pages/projects";
import ProjectsList from "@/components/pages/projects/ProjectsList.vue";
import { ref } from "vue";
import ProjectDialog from "@/components/pages/projects/ProjectDialog.vue";

// todo: fetch api
// todo: const { projects } = useProjects()
const projects: Project[] = [
  {
    id: 1,
    name: "amber_horizon",
    displayName: "Amber Horizon",
    path: "/home/user/projects/amber_horizon",
    createdAt: "2026-07-24T10:48:32+00:00",
    updatedAt: "2026-07-24T10:48:32+00:00",
    tasksCount: 7,
    agents: ["codex"],
  },
  {
    id: 2,
    name: "northforge",
    displayName: "Northforge",
    path: "/home/user/projects/northforge",
    createdAt: "2026-07-11T14:20:11+00:00",
    updatedAt: "2026-07-23T16:42:07+00:00",
    tasksCount: 23,
    agents: ["claude", "gemini"],
  },
  {
    id: 3,
    name: "opal_echo",
    displayName: "Opal Echo",
    path: "/home/user/projects/opal_echo",
    createdAt: "2026-06-30T08:51:39+00:00",
    updatedAt: "2026-07-22T19:15:10+00:00",
    tasksCount: 4,
    agents: ["codex", "claude"],
  },
  {
    id: 4,
    name: "iron_garden",
    displayName: "Iron Garden",
    path: "/home/user/projects/iron_garden",
    createdAt: "2026-05-18T11:12:56+00:00",
    updatedAt: "2026-07-24T07:10:25+00:00",
    tasksCount: 31,
    agents: ["gemini"],
  },
  {
    id: 5,
    name: "silent_orbit",
    displayName: "Silent Orbit",
    path: "/home/user/projects/silent_orbit",
    createdAt: "2026-07-02T17:08:44+00:00",
    updatedAt: "2026-07-24T09:42:16+00:00",
    tasksCount: 15,
    agents: ["codex", "gemini"],
  },
  {
    id: 6,
    name: "violet_drift",
    displayName: "Violet Drift",
    path: "/home/user/projects/violet_drift",
    createdAt: "2026-06-09T12:45:09+00:00",
    updatedAt: "2026-07-20T18:30:41+00:00",
    tasksCount: 9,
    agents: ["claude"],
  },
  {
    id: 7,
    name: "granite_loop",
    displayName: "Granite Loop",
    path: "/home/user/projects/granite_loop",
    createdAt: "2026-07-08T13:55:22+00:00",
    updatedAt: "2026-07-24T10:15:43+00:00",
    tasksCount: 42,
    agents: ["codex", "claude", "gemini"],
  },
  {
    id: 8,
    name: "polar_forge",
    displayName: "Polar Forge",
    path: "/home/user/projects/polar_forge",
    createdAt: "2026-05-26T16:03:17+00:00",
    updatedAt: "2026-07-23T08:21:58+00:00",
    tasksCount: 18,
    agents: ["codex"],
  },
  {
    id: 9,
    name: "ember_field",
    displayName: "Ember Field",
    path: "/home/user/projects/ember_field",
    createdAt: "2026-06-14T20:44:33+00:00",
    updatedAt: "2026-07-21T12:49:02+00:00",
    tasksCount: 5,
    agents: ["gemini", "claude"],
  },
  {
    id: 10,
    name: "crystal_harbor",
    displayName: "Crystal Harbor",
    path: "/home/user/projects/crystal_harbor",
    createdAt: "2026-07-04T10:07:19+00:00",
    updatedAt: "2026-07-24T10:31:27+00:00",
    tasksCount: 26,
    agents: ["codex", "claude"],
  },
  {
    id: 11,
    name: "atlas_bloom",
    displayName: "Atlas Bloom",
    path: "/home/user/projects/atlas_bloom",
    createdAt: "2026-06-01T09:16:08+00:00",
    updatedAt: "2026-07-18T21:40:15+00:00",
    tasksCount: 12,
    agents: ["claude"],
  },
  {
    id: 12,
    name: "shadow_weave",
    displayName: "Shadow Weave",
    path: "/home/user/projects/shadow_weave",
    createdAt: "2026-05-29T06:58:50+00:00",
    updatedAt: "2026-07-22T11:18:44+00:00",
    tasksCount: 37,
    agents: ["codex", "gemini"],
  },
  {
    id: 13,
    name: "nova_ridge",
    displayName: "Nova Ridge",
    path: "/home/user/projects/nova_ridge",
    createdAt: "2026-07-13T18:27:01+00:00",
    updatedAt: "2026-07-24T06:55:31+00:00",
    tasksCount: 14,
    agents: ["gemini"],
  },
  {
    id: 14,
    name: "lunar_anvil",
    displayName: "Lunar Anvil",
    path: "/home/user/projects/lunar_anvil",
    createdAt: "2026-06-22T14:37:52+00:00",
    updatedAt: "2026-07-23T20:12:29+00:00",
    tasksCount: 21,
    agents: ["codex", "claude"],
  },
  {
    id: 15,
    name: "oak_signal",
    displayName: "Oak Signal",
    path: "/home/user/projects/oak_signal",
    createdAt: "2026-07-16T07:24:13+00:00",
    updatedAt: "2026-07-24T10:39:54+00:00",
    tasksCount: 8,
    agents: ["codex"],
  },
];

const showProjectDialog = ref(false);

useTitle("Projects");
</script>

<template>
  <div class="mx-auto flex h-svh w-full max-w-5xl flex-col gap-4 overflow-hidden px-4 py-18">
    <div class="flex shrink-0 items-center justify-between">
      <div class="text-2xl font-bold">Projects</div>
      <Button @click="showProjectDialog = true">New project</Button>
    </div>

    <ProjectDialog v-model:open="showProjectDialog" />

    <div class="min-h-0 flex-1">
      <div class="flex max-h-full flex-col bg-card rounded-lg shadow ring-1 ring-foreground/5">
        <div class="min-h-0 overflow-y-auto custom-scrollbar">
          <EmptyProjectsList v-if="projects.length === 0" />
          <ProjectsList :projects="projects" />
        </div>

        <div v-if="projects.length > 0" class="shrink-0 border-t">
          <div class="p-3">footer</div>
        </div>
      </div>
    </div>
  </div>
</template>
