<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from "vue";
import NameConsole from "./components/name_console.vue";
import SignalPanel from "./components/signal_panel.vue";
import SettingsDrawer from "./components/settings_drawer.vue";
import GlyphIcon from "./components/glyph_icon.vue";
import {
  api,
  errorMessage,
  isActive,
  onLog,
  onState,
  type AppState,
  type Diagnostics,
  type LogLine,
} from "./api";

const fallbackState: AppState = {
  status: "idle",
  prefix: "my-device",
  suffix: "local",
  fqdn: "my-device.local",
  addresses: [],
  interfaces: [],
  selectedInterfaces: [],
  error: "",
  suggestion: "",
  since: 0,
  startedAt: 0,
  lastAnnounceAt: 0,
  queriesAnswered: 0,
  announcementsSent: 0,
  conflictsDetected: 0,
  autostartEnabled: false,
  autostartSupported: false,
  autostartStrategy: "",
  autoBroadcast: false,
  startMinimized: false,
  port: 5353,
  standardSuffix: true,
  testCommand: "",
  version: "0.1.0",
  platform: "Windows",
};

const state = ref<AppState | null>(null);
const logs = ref<LogLine[]>([]);
const diagnostics = ref<Diagnostics | null>(null);
const settingsOpen = ref(false);
const busy = ref(false);
const dirty = ref(false);
const toast = ref<{ message: string; kind: "info" | "error" } | null>(null);
const now = ref(Date.now());

const form = reactive({ prefix: "", suffix: "local" });

let unsubscribeState: (() => void) | undefined;
let unsubscribeLog: (() => void) | undefined;
let ticker: number | undefined;
let toastTimer: number | undefined;

const st = computed<AppState>(() => state.value ?? fallbackState);
const active = computed(() => isActive(st.value.status));

const statusChip = computed(() => {
  switch (st.value.status) {
    case "probing":
      return "探测中";
    case "announcing":
      return "宣告中";
    case "broadcasting":
      return "广播中";
    case "conflict":
      return "冲突";
    case "error":
      return "失败";
    default:
      return "待机";
  }
});

function syncForm(source: AppState) {
  form.prefix = source.prefix;
  form.suffix = source.suffix || "local";
}

function markDirty() {
  dirty.value = true;
}

function showToast(message: string, kind: "info" | "error" = "info") {
  toast.value = { message, kind };
  if (toastTimer) window.clearTimeout(toastTimer);
  toastTimer = window.setTimeout(() => {
    toast.value = null;
  }, 2600);
}

async function refresh() {
  try {
    const [next, history] = await Promise.all([api.GetState(), api.GetLogs()]);
    if (!state.value) {
      state.value = next;
    }
    if (!dirty.value) {
      syncForm(state.value ?? next);
    }
    if (logs.value.length === 0) {
      logs.value = (history ?? []).slice(-200);
    }
    diagnostics.value = diagnostics.value ?? (await api.GetDiagnostics());
  } catch (err) {
    showToast(errorMessage(err), "error");
  }
}

async function run<T>(task: () => PromiseLike<T>): Promise<T | undefined> {
  if (busy.value) return undefined;
  busy.value = true;
  try {
    return await task();
  } catch (err) {
    showToast(errorMessage(err), "error");
    return undefined;
  } finally {
    busy.value = false;
  }
}

async function start() {
  const result = await run(() =>
    api.StartBroadcast(form.prefix.trim(), form.suffix.trim() || "local"),
  );
  if (result) {
    state.value = result;
    syncForm(result);
    dirty.value = false;
  }
}

async function stop() {
  const result = await run(() => api.StopBroadcast());
  if (result) {
    state.value = result;
    syncForm(result);
    dirty.value = false;
  }
}

async function adoptSuggestion() {
  const applied = await run<AppState>(() => api.UseSuggestedName());
  if (!applied) return;
  state.value = applied;
  form.prefix = applied.prefix;
  form.suffix = applied.suffix;
  const started = await run(() => api.StartBroadcast(applied.prefix, applied.suffix));
  if (started) {
    state.value = started;
    syncForm(started);
    dirty.value = false;
  }
}

async function copy(text?: string) {
  const value = text || st.value.fqdn;
  if (!value) return;
  try {
    await api.CopyText(value);
    showToast(`已复制 · ${value}`);
  } catch (err) {
    showToast(errorMessage(err), "error");
  }
}

async function openSettings() {
  settingsOpen.value = true;
  try {
    diagnostics.value = await api.GetDiagnostics();
  } catch (err) {
    showToast(errorMessage(err), "error");
  }
}

async function toggleAutostart() {
  if (!st.value.autostartSupported) {
    showToast("当前平台不支持开机自启", "error");
    return;
  }
  const enabled = !st.value.autostartEnabled;
  const result = await run(() => api.SetAutostart(enabled));
  if (result === undefined) return;
  const next = await api.GetState();
  state.value = next;
  showToast(enabled ? "已注册开机自启" : "已关闭开机自启");
}

async function toggleAutoBroadcast() {
  const result = await run(() => api.SetAutoBroadcast(!st.value.autoBroadcast));
  if (result) state.value = result;
}

async function toggleStartMinimized() {
  const result = await run(() => api.SetStartMinimized(!st.value.startMinimized));
  if (result) state.value = result;
}

async function applyInterfaces(indexes: number[]) {
  const result = await run(() => api.SetInterfaces(indexes));
  if (result) {
    state.value = result;
    showToast("网络接口已更新");
  }
}

function minimiseWindow() {
  void api.MinimiseWindow();
}

function hideWindow() {
  void api.HideWindow();
}

function onGlobalKey(event: KeyboardEvent) {
  if (event.key === "Escape" && settingsOpen.value) {
    settingsOpen.value = false;
  }
}

onMounted(async () => {
  unsubscribeState = onState((next) => {
    state.value = next;
    if (!dirty.value) {
      syncForm(next);
    }
  });
  unsubscribeLog = onLog((line) => {
    logs.value = [...logs.value, line].slice(-200);
  });
  window.addEventListener("keydown", onGlobalKey);
  ticker = window.setInterval(() => {
    now.value = Date.now();
  }, 1000);
  await refresh();
});

onBeforeUnmount(() => {
  unsubscribeState?.();
  unsubscribeLog?.();
  window.removeEventListener("keydown", onGlobalKey);
  if (ticker) window.clearInterval(ticker);
  if (toastTimer) window.clearTimeout(toastTimer);
});
</script>

<template>
  <div class="app-shell" :data-state="st.status" :data-platform="st.platform">
    <header class="topbar">
      <div class="brand">
        <span class="brand-mark">
          <GlyphIcon name="wifi" :size="18" />
        </span>
        <span class="wordmark">LAN <span class="accent">NAME</span></span>
        <span class="overline">Atelier de signal · 2026</span>
      </div>

      <div class="top-actions">
        <span class="state-chip" aria-live="polite">
          <i class="led" />
          {{ statusChip }}
        </span>
        <button class="icon-btn" type="button" title="设置" aria-label="设置" @click="openSettings">
          <GlyphIcon name="settings" :size="17" />
        </button>
        <button
          class="icon-btn"
          type="button"
          title="最小化"
          aria-label="最小化"
          @click="minimiseWindow"
        >
          <GlyphIcon name="minus" :size="17" />
        </button>
        <button
          class="icon-btn"
          type="button"
          title="隐藏到托盘（不会退出）"
          aria-label="隐藏到托盘"
          @click="hideWindow"
        >
          <GlyphIcon name="close" :size="16" />
        </button>
      </div>
    </header>

    <main class="stage">
      <NameConsole
        :prefix="form.prefix"
        :suffix="form.suffix"
        :fqdn="st.fqdn"
        :status="st.status"
        :error="st.error"
        :suggestion="st.suggestion"
        :busy="busy"
        :started-at="st.startedAt"
        @update:prefix="
          (value) => {
            form.prefix = value;
            markDirty();
          }
        "
        @update:suffix="
          (value) => {
            form.suffix = value;
            markDirty();
          }
        "
        @start="start"
        @stop="stop"
        @adopt="adoptSuggestion"
        @copy="copy()"
      />

      <SignalPanel :state="st" :logs="logs" :now="now" />
    </main>

    <footer class="statusline">
      <span>UDP {{ st.port }}</span>
      <span class="opt-iface">IFACE {{ (st.selectedInterfaces ?? []).length || (st.interfaces ?? []).length }}</span>
      <span class="hot">{{ st.fqdn }}</span>
      <span class="spacer" />
      <span :class="{ live: st.status === 'broadcasting' }">
        {{ st.status === "broadcasting" ? "SIGNAL LOCKED" : "SIGNAL STANDBY" }}
      </span>
      <span class="opt-autostart">AUTOSTART {{ st.autostartEnabled ? "ON" : "OFF" }}</span>
      <span>V{{ st.version }}</span>
    </footer>

    <SettingsDrawer
      :open="settingsOpen"
      :state="st"
      :diagnostics="diagnostics"
      @close="settingsOpen = false"
      @toggle-autostart="toggleAutostart"
      @toggle-auto-broadcast="toggleAutoBroadcast"
      @toggle-start-minimized="toggleStartMinimized"
      @apply-interfaces="applyInterfaces"
      @open-folder="api.OpenLogFolder()"
      @copy="copy"
    />

    <Transition name="toast">
      <div v-if="toast" class="toast" :class="toast.kind" role="status">
        {{ toast.message }}
      </div>
    </Transition>
  </div>
</template>
