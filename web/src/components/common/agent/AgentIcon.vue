<script lang="ts" setup="">
import LogosOpenaiIcon from "~icons/logos/openai-icon";
import LogosClaudeIcon from "~icons/logos/claude-icon";
import VscodeIconsFileTypeGemini from "~icons/vscode-icons/file-type-gemini";
import VscodeIconsFileTypeCursorrules from "~icons/vscode-icons/file-type-cursorrules";
import SelfhstMicrosoftCopilot from "~icons/selfhst/microsoft-copilot";
import LogosGrokIcon from "~icons/logos/grok-icon";
import MynauiTerminalSolid from "~icons/mynaui/terminal-solid";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils.ts";

const props = defineProps<{
  value?: string | "openai" | "codex" | "claude" | "gemini" | "cursor" | "copilot" | "grok";
  class?: any;
}>();

const commonClasss = "rounded-full size-6 flex items-center justify-center";
</script>

<template>
  <TooltipProvider>
    <Tooltip>
      <TooltipTrigger>
        <div
          v-if="props.value === 'codex' || props.value === 'openai'"
          :class="cn('bg-foreground', commonClasss, props.class)"
        >
          <LogosOpenaiIcon class="size-4" />
        </div>

        <div
          v-else-if="props.value === 'claude'"
          :class="cn('bg-foreground', commonClasss, props.class)"
        >
          <LogosClaudeIcon class="size-4" />
        </div>

        <div
          v-else-if="props.value === 'gemini'"
          :class="cn('bg-foreground', commonClasss, props.class)"
        >
          <VscodeIconsFileTypeGemini class="size-4" />
        </div>

        <div
          v-else-if="props.value === 'cursor'"
          :class="cn('bg-foreground', commonClasss, props.class)"
        >
          <VscodeIconsFileTypeCursorrules class="size-4" />
        </div>

        <div
          v-else-if="props.value === 'copilot'"
          :class="cn('bg-foreground', commonClasss, props.class)"
        >
          <SelfhstMicrosoftCopilot class="size-4" />
        </div>

        <div
          v-else-if="props.value === 'grok'"
          :class="cn('bg-foreground', commonClasss, props.class)"
        >
          <LogosGrokIcon class="size-4" />
        </div>

        <div v-else :class="cn('bg-foreground', commonClasss, props.class)">
          <MynauiTerminalSolid class="size-4 text-background" />
        </div>
      </TooltipTrigger>
      <TooltipContent>
        <p>{{ props.value || "Generic agent" }}</p>
      </TooltipContent>
    </Tooltip>
  </TooltipProvider>
</template>
