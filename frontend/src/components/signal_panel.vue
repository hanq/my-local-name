<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue";
import type { AppState, LogLine } from "../api";

const props = defineProps<{
  state: AppState;
  logs: LogLine[];
  now: number;
}>();

const logBody = ref<HTMLElement | null>(null);

const addresses = computed(() => props.state.addresses ?? []);
const interfaces = computed(() => props.state.interfaces ?? []);

const ipRows = computed(() =>
  addresses.value.map((ip) => {
    const iface = interfaces.value.find((item) => (item.addresses ?? []).includes(ip));
    return { ip, iface: iface?.name ?? "" };
  }),
);

const visibleLogs = computed(() => props.logs.slice(-5));

const uptime = computed(() => {
  if (!props.state.startedAt) return "00:00";
  const seconds = Math.max(0, Math.floor((props.now - props.state.startedAt) / 1000));
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;
  const pad = (n: number) => String(n).padStart(2, "0");
  return h > 0 ? `${h}:${pad(m)}:${pad(s)}` : `${pad(m)}:${pad(s)}`;
});

const caption = computed(() => {
  switch (props.state.status) {
    case "broadcasting":
      return "SIGNAL LOCKED · 每 60s 重宣告";
    case "probing":
      return "PROBING · 检查名字冲突";
    case "announcing":
      return "ANNOUNCING · 发送 A 记录";
    case "conflict":
      return "NAME CONFLICT · 已停止";
    case "error":
      return "BIND FAILED · 查看日志";
    default:
      return "NO SIGNAL · 尚未广播";
  }
});

const emptyText = computed(() => {
  switch (props.state.status) {
    case "probing":
      return "正在探测同名设备…";
    case "announcing":
      return "探测通过，正在宣告…";
    case "conflict":
      return "名称冲突，未广播";
    case "error":
      return "启动失败，未广播";
    default:
      return "启动后在此显示 A 记录地址";
  }
});

const tagText = computed(() => {
  switch (props.state.status) {
    case "broadcasting":
      return "LIVE";
    case "probing":
    case "announcing":
      return "SYNC";
    case "conflict":
    case "error":
      return "FAULT";
    default:
      return "IDLE";
  }
});

watch(
  () => props.logs.length,
  async () => {
    await nextTick();
    if (logBody.value) {
      logBody.value.scrollTop = logBody.value.scrollHeight;
    }
  },
);
</script>

<template>
  <section class="panel panel-signal" aria-labelledby="signal-title">
    <div class="sig-head">
      <span id="signal-title" class="sig-label">SIGNAL</span>
      <span class="sig-tag live">{{ tagText }}</span>
    </div>

    <div class="sig-status">
      <div class="sig-orb" aria-hidden="true">
        <span class="sig-sweep" />
        <span class="sig-core" />
      </div>
      <div class="sig-id">
        <div class="sig-fqdn">{{ state.fqdn || "—" }}</div>
        <div class="sig-caption">{{ caption }}</div>
      </div>
    </div>

    <div class="ip-chips">
      <span v-for="row in ipRows" :key="row.ip" class="ip-chip">
        <i>A</i>
        <span>{{ row.ip }}</span>
        <em v-if="row.iface">{{ row.iface }}</em>
      </span>
      <span v-if="ipRows.length === 0" class="ip-empty">{{ emptyText }}</span>
    </div>

    <div class="metrics">
      <div class="metric">
        <b>{{ state.queriesAnswered }}</b>
        <span>查询</span>
      </div>
      <div class="metric">
        <b>{{ state.announcementsSent }}</b>
        <span>宣告</span>
      </div>
      <div class="metric">
        <b>{{ state.conflictsDetected }}</b>
        <span>冲突</span>
      </div>
      <div class="metric">
        <b>{{ uptime }}</b>
        <span>运行</span>
      </div>
    </div>

    <div class="log-panel">
      <div class="log-head">
        <span>Packet log</span>
        <span>{{ addresses.length }} addr · TTL 120s</span>
      </div>
      <div ref="logBody" class="log-body">
        <div v-for="line in visibleLogs" :key="line.id" class="log-line">
          <span class="log-time">{{ line.time }}</span>
          <span class="log-level" :class="line.level">{{ line.level }}</span>
          <span class="log-msg">{{ line.message }}</span>
        </div>
        <div v-if="visibleLogs.length === 0" class="log-empty">
          等待第一个数据包…
        </div>
      </div>
    </div>
  </section>
</template>
