<script setup lang="ts">
withDefaults(defineProps<{ corner?: string }>(), { corner: "tl" });
</script>

<template>
  <svg
    class="ornament"
    :class="`ornament-${corner}`"
    viewBox="0 0 120 120"
    fill="none"
    stroke="currentColor"
    stroke-width="1.4"
    stroke-linecap="round"
    aria-hidden="true"
  >
    <path d="M3 117V56c0-29 23-52 52-52h62" />
    <path d="M15 117V58c0-24 19-43 43-43h62" opacity=".55" />
    <path d="M31 99c0-16 13-29 29-29" opacity=".8" />
    <path d="M15 117c0-9 7-16 16-16" opacity=".6" />
    <circle cx="3" cy="117" r="2.2" fill="currentColor" stroke="none" />
  </svg>
</template>
