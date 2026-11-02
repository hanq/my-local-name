<script setup lang="ts">
withDefaults(
  defineProps<{
    name: string;
    size?: number;
  }>(),
  { size: 20 },
);

const paths: Record<string, string> = {
  play: '<path d="M8 5.5 18.5 12 8 18.5V5.5Z"/>',
  stop: '<rect x="6" y="6" width="12" height="12" rx="1.5"/>',
  copy: '<rect x="9" y="9" width="11" height="11" rx="1.5"/><path d="M15 5H6a1 1 0 0 0-1 1v9"/>',
  settings:
    '<path d="M4 7h8M17 7h3M4 17h3M12 17h8"/><circle cx="14.5" cy="7" r="2.5"/><circle cx="9.5" cy="17" r="2.5"/>',
  minus: '<path d="M5 12h14"/>',
  close: '<path d="M6 6l12 12M18 6 6 18"/>',
  wifi: '<path d="M3 9a15 15 0 0 1 18 0M6.5 12.6a10 10 0 0 1 11 0M10 16.2a5 5 0 0 1 4 0"/><circle cx="12" cy="19.4" r="1" fill="currentColor" stroke="none"/>',
  network:
    '<circle cx="12" cy="5" r="2"/><circle cx="5" cy="19" r="2"/><circle cx="19" cy="19" r="2"/><path d="M12 7v4M12 11l-5 6M12 11l5 6"/>',
  clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
  check: '<path d="M5 12l5 5L20 7"/>',
  alert: '<path d="M12 3l9 17H3L12 3Z"/><path d="M12 9v4M12 16.2v.3"/>',
  arrow: '<path d="M4 12h16M14 6l6 6-6 6"/>',
  folder: '<path d="M3 7a2 2 0 0 1 2-2h4l2 3h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7Z"/>',
  terminal: '<path d="M4 5h16v14H4z"/><path d="M7 10l3 2-3 2M13 14h4"/>',
  shield: '<path d="M12 3l8 3v6c0 5-4 8-8 9-4-1-8-4-8-9V6l8-3Z"/><path d="M9 12l2 2 4-4"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v6M12 7.2v.3"/>',
  power: '<path d="M12 3v9"/><path d="M7 6a8 8 0 1 0 10 0"/>',
  radar: '<circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="5"/><circle cx="12" cy="12" r="1.4" fill="currentColor" stroke="none"/>',
  monitor: '<rect x="3" y="4" width="18" height="12" rx="1.5"/><path d="M8 20h8M12 16v4"/>',
  chevron: '<path d="M9 5l7 7-7 7"/>',
};
</script>

<template>
  <svg
    :width="size"
    :height="size"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="2"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
    focusable="false"
    v-html="paths[name] ?? paths.info"
  />
</template>
