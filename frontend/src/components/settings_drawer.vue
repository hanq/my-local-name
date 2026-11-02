<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { AppState, Diagnostics } from "../api";
import GlyphIcon from "./glyph_icon.vue";

const props = defineProps<{
  open: boolean;
  state: AppState;
  diagnostics: Diagnostics | null;
}>();

const emit = defineEmits<{
  close: [];
  "toggle-autostart": [];
  "toggle-auto-broadcast": [];
  "toggle-start-minimized": [];
  "apply-interfaces": [indexes: number[]];
  "open-folder": [];
  copy: [text: string];
}>();

const selected = ref<number[]>([]);
const interfaces = computed(() => props.state.interfaces ?? []);

function syncSelection() {
  selected.value = interfaces.value
    .filter((item) => item.selected)
    .map((item) => item.index);
}

watch(
  () => props.open,
  (open) => {
    if (open) syncSelection();
  },
);

watch(
  () => interfaces.value,
  () => {
    if (props.open) syncSelection();
  },
  { deep: true },
);

const dirty = computed(() => {
  const current = [...selected.value].sort((a, b) => a - b);
  const original = interfaces.value
    .filter((item) => item.selected)
    .map((item) => item.index)
    .sort((a, b) => a - b);
  return current.join(",") !== original.join(",");
});

const strategyText = computed(() => {
  switch (props.state.autostartStrategy) {
    case "registry-run":
      return "Windows · HKCU Run";
    case "smappservice":
      return "macOS · SMAppService";
    case "launchagent":
      return "macOS · LaunchAgent";
    case "xdg-autostart":
      return "Linux · XDG autostart";
    default:
      return "未注册";
  }
});

function toggleIface(index: number) {
  if (selected.value.includes(index)) {
    selected.value = selected.value.filter((i) => i !== index);
  } else {
    selected.value = [...selected.value, index];
  }
}

function applyInterfaces() {
  if (!dirty.value || selected.value.length === 0) return;
  emit("apply-interfaces", [...selected.value]);
}
</script>

<template>
  <div>
    <div class="drawer-overlay" :class="{ open }" @click="emit('close')" />

    <aside class="drawer" :class="{ open }" aria-label="设置" :aria-hidden="!open">
      <div class="drawer-head">
        <div>
          <h2 class="drawer-title">设置</h2>
          <p class="drawer-kicker">Atelier settings · v{{ state.version }}</p>
        </div>
        <button class="icon-btn" type="button" aria-label="关闭设置" @click="emit('close')">
          <GlyphIcon name="close" :size="16" />
        </button>
      </div>

      <div class="drawer-body">
        <section class="settings-group">
          <h3 class="group-label">Behaviour / 行为</h3>

          <div class="setting-row">
            <div>
              <div class="setting-name">开机自启动</div>
              <div class="setting-desc">
                {{ state.autostartSupported ? strategyText : "当前平台不支持开机自启" }}
              </div>
            </div>
            <button
              class="toggle"
              :class="{ on: state.autostartEnabled }"
              type="button"
              role="switch"
              :aria-checked="state.autostartEnabled"
              :disabled="!state.autostartSupported"
              aria-label="开机自启动"
              @click="emit('toggle-autostart')"
            />
          </div>

          <div class="setting-row">
            <div>
              <div class="setting-name">自启后自动广播</div>
              <div class="setting-desc">登录后静默驻留托盘，并广播上次保存的名字。</div>
            </div>
            <button
              class="toggle"
              :class="{ on: state.autoBroadcast }"
              type="button"
              role="switch"
              :aria-checked="state.autoBroadcast"
              aria-label="自启后自动广播"
              @click="emit('toggle-auto-broadcast')"
            />
          </div>

          <div class="setting-row">
            <div>
              <div class="setting-name">启动时最小化到托盘</div>
              <div class="setting-desc">手动启动时不弹窗，只留托盘图标。</div>
            </div>
            <button
              class="toggle"
              :class="{ on: state.startMinimized }"
              type="button"
              role="switch"
              :aria-checked="state.startMinimized"
              aria-label="启动时最小化到托盘"
              @click="emit('toggle-start-minimized')"
            />
          </div>
        </section>

        <section class="settings-group">
          <h3 class="group-label">Interfaces / 网络接口</h3>

          <div v-for="iface in interfaces" :key="iface.index" class="iface-item">
            <button
              class="iface-check"
              :class="{ on: selected.includes(iface.index) }"
              type="button"
              role="checkbox"
              :aria-checked="selected.includes(iface.index)"
              :aria-label="`选择接口 ${iface.name}`"
              @click="toggleIface(iface.index)"
            >
              <GlyphIcon v-if="selected.includes(iface.index)" name="check" :size="11" />
            </button>
            <div>
              <div class="iface-name">{{ iface.name }}</div>
              <div class="iface-addrs">{{ (iface.addresses ?? []).join(" · ") }}</div>
            </div>
          </div>

          <div v-if="interfaces.length === 0" class="diag-note">
            没有检测到可用的局域网接口。
          </div>

          <div class="drawer-footer">
            <button
              class="btn-ghost"
              :class="{ solid: dirty }"
              type="button"
              :disabled="!dirty || selected.length === 0"
              @click="applyInterfaces"
            >
              应用接口
            </button>
          </div>
        </section>

        <section class="settings-group">
          <h3 class="group-label">Diagnostics / 诊断</h3>

          <div class="diag-row">
            <span class="diag-key">fqdn</span>
            <span class="diag-val">{{ state.fqdn || "—" }}</span>
          </div>
          <div class="diag-row">
            <span class="diag-key">port</span>
            <span class="diag-val">{{ state.port }}/udp · 224.0.0.251</span>
          </div>
          <div class="diag-row">
            <span class="diag-key">status</span>
            <span class="diag-val">{{ state.status }}</span>
          </div>

          <template v-if="diagnostics">
            <button
              v-for="command in diagnostics.commands"
              :key="command"
              class="diag-code"
              type="button"
              @click="emit('copy', command)"
            >
              <span>{{ command }}</span>
              <GlyphIcon name="copy" :size="11" />
            </button>

            <div class="diag-row" style="margin-top: 8px">
              <span class="diag-key">config</span>
              <span class="diag-val">{{ diagnostics.configPath }}</span>
            </div>
            <div class="diag-row">
              <span class="diag-key">log</span>
              <span class="diag-val">{{ diagnostics.logPath }}</span>
            </div>

            <p
              v-for="note in diagnostics.notes"
              :key="note"
              class="diag-note"
              :class="{ 'danger-note': !diagnostics.suffixStandard && note.includes('自定义后缀') }"
            >
              {{ note }}
            </p>
          </template>

          <div class="drawer-footer">
            <button class="btn-ghost" type="button" @click="emit('open-folder')">
              <GlyphIcon name="folder" :size="12" style="vertical-align: -2px" />
              打开日志目录
            </button>
          </div>
        </section>
      </div>
    </aside>
  </div>
</template>
