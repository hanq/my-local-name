<script setup lang="ts">
import { computed } from "vue";
import BaroqueOrnament from "./baroque_ornament.vue";
import GlyphIcon from "./glyph_icon.vue";

const props = defineProps<{
  prefix: string;
  suffix: string;
  fqdn: string;
  status: string;
  error: string;
  suggestion: string;
  busy: boolean;
  startedAt: number;
}>();

const emit = defineEmits<{
  "update:prefix": [value: string];
  "update:suffix": [value: string];
  start: [];
  stop: [];
  adopt: [];
  copy: [];
}>();

const cleanSuffix = computed(() =>
  (props.suffix || "local")
    .trim()
    .toLowerCase()
    .replace(/^\.+/, "")
    .replace(/\.$/, "") || "local",
);

const localError = computed(() => {
  const prefix = (props.prefix || "").trim().toLowerCase();
  const suffix = cleanSuffix.value;
  const label = /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/;

  if (!prefix) return "先给它一个名字";
  if (/\s/.test(prefix)) return "名字不能包含空格";
  if (prefix.includes(".")) return "前缀不能包含点号，把后半段填到后缀里";
  if (prefix.length > 63) return "前缀最多 63 字节";
  if (!label.test(prefix)) return "只能使用小写字母、数字和连字符，且不能以连字符开头或结尾";

  for (const part of suffix.split(".")) {
    if (!part) return "后缀不能包含连续的点";
    if (part.length > 63) return "后缀每段最多 63 字节";
    if (!label.test(part)) return "后缀只能使用小写字母、数字和连字符";
  }
  if (`${prefix}.${suffix}`.length > 253) return "完整名字过长（最多 253 字节）";
  return "";
});

const shownError = computed(() => localError.value || props.error);
const isBroadcasting = computed(() => props.status === "broadcasting");
const isTransitioning = computed(
  () => props.status === "probing" || props.status === "announcing",
);
const isActive = computed(() => isBroadcasting.value || isTransitioning.value);
const canStart = computed(() => !isActive.value && !localError.value && !props.busy);

const buttonLabel = computed(() => {
  if (isBroadcasting.value) return "停止广播";
  if (props.status === "probing") return "正在探测…";
  if (props.status === "announcing") return "正在宣告…";
  if (props.busy) return "正在启动…";
  return "开始广播";
});

const statusText = computed(() => {
  switch (props.status) {
    case "probing":
      return "正在探测名字冲突 · 最多 750ms";
    case "announcing":
      return "正在宣告 A 记录 · 224.0.0.251";
    case "broadcasting": {
      const clock = props.startedAt ? formatClock(props.startedAt) : "";
      return clock ? `广播中 · 自 ${clock} 起` : "广播中 · 每 60 秒重宣告";
    }
    case "conflict":
      return "名称冲突 · 已停止广播";
    case "error":
      return "启动失败 · 详情见下方日志";
    default:
      return "待机 · 尚未广播";
  }
});

const suggestionFqdn = computed(() => {
  if (!props.suggestion) return "";
  return `${props.suggestion}.${cleanSuffix.value}`;
});

function formatClock(ms: number): string {
  const d = new Date(ms);
  return [d.getHours(), d.getMinutes(), d.getSeconds()]
    .map((n) => String(n).padStart(2, "0"))
    .join(":");
}

function onPrefix(event: Event) {
  emit("update:prefix", (event.target as HTMLInputElement).value);
}

function onSuffix(event: Event) {
  emit("update:suffix", (event.target as HTMLInputElement).value);
}

function onPrimary() {
  if (isBroadcasting.value) {
    emit("stop");
    return;
  }
  if (canStart.value) emit("start");
}

function onEnter() {
  if (isBroadcasting.value) {
    emit("stop");
    return;
  }
  if (canStart.value) emit("start");
}
</script>

<template>
  <section class="panel panel-cream" aria-labelledby="name-title">
    <BaroqueOrnament corner="tl" />
    <BaroqueOrnament corner="br" />

    <div class="cn-content">
      <div class="cn-overline">Name your machine · mDNS</div>

      <h1 id="name-title" class="cn-title">
        局域网<em>名字</em>
      </h1>
      <p class="cn-sub">
        同一网络里的设备，可用
        <code>{{ fqdn || "name.local" }}</code>
        找到这台机器。
      </p>

      <div class="cn-form">
        <div class="nameplate" :class="{ 'has-error': !!localError }">
          <input
            class="name-input"
            :value="prefix"
            :disabled="isActive"
            maxlength="63"
            placeholder="hanq-pi"
            aria-label="主机名前缀"
            autocomplete="off"
            autocapitalize="off"
            spellcheck="false"
            @input="onPrefix"
            @keydown.enter.prevent="onEnter"
          />
          <span class="name-suffix">
            <span class="dot">.</span>
            <input
              class="suffix-input"
              :value="cleanSuffix"
              :disabled="isActive"
              maxlength="189"
              aria-label="主机名后缀"
              autocomplete="off"
              autocapitalize="off"
              spellcheck="false"
              @input="onSuffix"
              @keydown.enter.prevent="onEnter"
            />
          </span>
        </div>

        <div class="cn-meta">
          <span v-if="shownError" class="cn-error">
            <GlyphIcon name="alert" :size="11" />
            {{ shownError }}
          </span>
          <span v-else>小写字母 · 数字 · 连字符；后缀默认 .local</span>
          <span>{{ fqdn.length }}/253</span>
        </div>

        <div v-if="cleanSuffix !== 'local'" class="cn-warning">
          <GlyphIcon name="info" :size="11" />
          <span>
            自定义后缀已在协议层广播；Windows / macOS 原生解析器通常只处理
            <code>.local</code>。
          </span>
        </div>

        <div class="cn-actions">
          <button
            class="btn-primary"
            :class="{ 'is-stop': isBroadcasting }"
            :disabled="isBroadcasting ? busy : !canStart"
            :aria-busy="busy"
            @click="onPrimary"
          >
            <GlyphIcon :name="isBroadcasting ? 'stop' : 'play'" :size="14" />
            {{ buttonLabel }}
          </button>
          <button
            class="btn-copy"
            :disabled="!fqdn"
            title="复制完整地址"
            @click="emit('copy')"
          >
            <GlyphIcon name="copy" :size="12" />
            复制地址
          </button>
        </div>

        <div class="cn-status" aria-live="polite">
          <span class="dash" />
          <span>{{ statusText }}</span>
        </div>

        <div v-if="status === 'conflict' && suggestion" class="conflict-note" role="alert">
          <span>
            建议改用 <span class="suggestion">{{ suggestionFqdn }}</span>
          </span>
          <button type="button" @click="emit('adopt')">采用并重试</button>
        </div>
      </div>

      <div class="cn-foot">mDNS · RFC 6762 · UDP 5353 · 224.0.0.251</div>
    </div>
  </section>
</template>
