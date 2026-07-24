<script setup lang="ts">
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty";
import { Button } from "@/components/ui/button";

const goToProjects = () => window.location.replace("/projects");
</script>

<template>
  <Empty class="h-svh overflow-hidden bg-background">
    <EmptyHeader>
      <EmptyTitle>404 - Not Found</EmptyTitle>
      <EmptyDescription>
        The page you're looking for doesn't exist. Try searching for what you need below.
      </EmptyDescription>
    </EmptyHeader>
    <EmptyContent>
      <Button @click="goToProjects">Go to projects</Button>
    </EmptyContent>
  </Empty>
</template>
