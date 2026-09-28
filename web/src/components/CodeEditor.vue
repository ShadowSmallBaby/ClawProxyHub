<template>
  <div class="code-editor" :style="{ height }">
    <!-- 挂载点：CodeMirror 就绪后显示，否则回退 textarea -->
    <div ref="cmRef" v-show="ready" class="ce-cm"></div>
    <template v-if="!ready">
      <div ref="gutterRef" class="ce-gutter">
        <div v-for="n in lineCount" :key="n" class="ce-ln">{{ n }}</div>
      </div>
      <textarea
        ref="taRef"
        class="ce-ta"
        :value="modelValue"
        spellcheck="false"
        wrap="off"
        @input="onInput"
        @scroll="onScroll"
      />
    </template>
  </div>
</template>

<script setup lang="ts">
// 代码编辑器：CodeMirror 6（Lua 高亮 + 补全），懒加载，失败回退增强 textarea。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'

const props = withDefaults(defineProps<{ modelValue: string; height?: string }>(), { height: '460px' })
const emit = defineEmits<{ (e: 'update:modelValue', v: string): void }>()

const cmRef = ref<HTMLDivElement>()
const taRef = ref<HTMLTextAreaElement>()
const gutterRef = ref<HTMLDivElement>()
const ready = ref(false)

// 回退 textarea 行号
const lineCount = computed(() => Math.max(props.modelValue.split('\n').length, 1))

let view: any = null
let syncFromCM = false

// cph.* 宿主能力全集（与 hosts/luahost 对齐）
const CPH_DOCS: Record<string, string> = {
  'cph.http.request': '发一次 HTTP 请求 → {body, status, headers}；失败 raise（pcall 接）。opts: {method, url, headers, body, timeout}',
  'cph.http.stream': 'SSE 流式请求。format="openai" 时宿主解析并驱动 stream 对象；4xx/5xx 回 (false, "HTTP <code>: ...")',
  'cph.json.encode': 'table → JSON 字符串',
  'cph.json.decode': 'JSON 字符串 → table',
  'cph.hash.md5': 'MD5 摘要（hex）',
  'cph.hash.sha256': 'SHA-256 摘要（hex）',
  'cph.hash.hmac_sha256': 'HMAC-SHA256(data, key)（hex）',
  'cph.hash.base64url_encode': 'Base64URL 编码（无 padding）',
  'cph.hash.base64url_decode': 'Base64URL 解码（兼容 padding/标准字母表）',
  'cph.time.now': '当前毫秒时间戳',
  'cph.time.sleep': '阻塞当前 VM 指定毫秒（重试退避用）',
  'cph.random.uuid': '随机 UUID 字符串',
  'cph.random.hex': '随机 hex 字符串（参数为字节数，如 hex(32) = 64 位）',
  'cph.openai.chat_body': 'ChatRequest 信封 → OpenAI chat body（table，可再改 model 等）',
  'cph.log.debug': '调试日志（fields 可选）',
  'cph.log.info': '信息日志',
  'cph.log.warn': '警告日志',
  'cph.log.error': '错误日志',
}

// 约定函数与 stream 事件
const SNIPPETS = [
  { label: 'plugin.handshake', detail: '约定函数', apply: 'function plugin.handshake(req)\n  \nend\n', info: '握手：返回 {manifest={...}} 声明能力（契约 ≥2 时优先于 manifest.json）' },
  { label: 'plugin.models', detail: '约定函数', apply: 'function plugin.models(cred)\n  return { models = { { id = "", context_window = 32768, supports_tools = true, supports_stream = true } } }\nend\n', info: '模型目录：返回 {models={{id, context_window, supports_tools, supports_stream}}}' },
  { label: 'plugin.login', detail: '约定函数', apply: 'function plugin.login(req)\n  \nend\n', info: '登录：req={method_id, form, state}；返回 {blob, profile} 或 {next={...}} 或 {error}' },
  { label: 'plugin.chat', detail: '约定函数', apply: 'function plugin.chat(req, stream)\n  \nend\n', info: '对话：req={model, messages, credential...}；用 stream 对象发事件' },
  { label: 'plugin.refresh', detail: '约定函数', apply: 'function plugin.refresh(cred)\n  \nend\n', info: '刷新凭据：返回 {blob, profile} 或 {error}' },
  { label: 'plugin.profile', detail: '约定函数', apply: 'function plugin.profile(cred)\n  \nend\n', info: '账号档案：返回 {display_name, healthy, quota, credits_json}' },
  { label: 'plugin.tasks', detail: '约定函数', apply: 'function plugin.tasks()\n  return { capabilities = {\n    { id = "", label = { zh = "" }, kind = "recurring", per_account = true, default_schedule = "daily 09:00" },\n  } }\nend\n', info: '任务能力声明：kind=once/recurring/both；per_account 按账号逐个执行；default_schedule 推荐调度' },
  { label: 'plugin.task', detail: '约定函数', apply: 'function plugin.task(req)\n  \nend\n', info: '任务执行：req={capability_id, credential, context}；返回 {summary, changed, blob, detail_json, notification, error}' },
  { label: 'stream.message_start', detail: 'stream 事件', apply: 'stream.message_start({ model = req.model })\n', info: '消息开始事件（只发一次）' },
  { label: 'stream.content_delta', detail: 'stream 事件', apply: 'stream.content_delta({ text = "" })\n', info: '正文增量' },
  { label: 'stream.reasoning_delta', detail: 'stream 事件', apply: 'stream.reasoning_delta({ text = "", signature = "" })\n', info: '思考增量' },
  { label: 'stream.tool_call_delta', detail: 'stream 事件', apply: 'stream.tool_call_delta({ id = "", name = "", arguments_delta = "" })\n', info: '工具调用增量' },
  { label: 'stream.message_finish', detail: 'stream 事件', apply: 'stream.message_finish({ finish_reason = "stop", usage = {} })\n', info: '消息结束（finish_reason: stop/end_turn/tool_use）' },
  { label: 'stream.failed', detail: 'stream 事件', apply: 'stream.failed({ code = 502, message = "" })\n', info: '失败事件（401 交给网关恢复链路，勿吞）' },
]

// 静态补全源：cph.* + 约定函数 + stream 事件
function buildCompletionSource() {
  return (ctx: any) => {
    const line = ctx.state.doc.lineAt(ctx.pos)
    const before = line.text.slice(0, ctx.pos - line.from)
    const m = /[\w.]*$/.exec(before)
    const word = m ? m[0] : ''
    if (!word) return null
    const options = [
      ...Object.entries(CPH_DOCS).map(([label, info]) => ({
        label, type: 'namespace', info,
      })),
      ...SNIPPETS.map((s) => ({ label: s.label, type: 'function', detail: s.detail, apply: s.apply, info: s.info })),
      { label: 'local function', type: 'keyword', apply: 'local function ', info: '局部函数定义' },
      { label: 'return plugin', type: 'keyword', apply: 'return plugin\n', info: '脚本末尾返回 module table（约定）' },
    ]
    return { from: ctx.pos - word.length, options, validFor: /^[\w.]*$/ }
  }
}

async function mountCM() {
  const [cmLang, cmView, cmCmds, cmAuto, cmSearch, legacy] = await Promise.all([
    import('@codemirror/language'),
    import('@codemirror/view'),
    import('@codemirror/commands'),
    import('@codemirror/autocomplete'),
    import('@codemirror/search'),
    import('@codemirror/legacy-modes/mode/lua'),
  ])
  const { StreamLanguage, syntaxHighlighting, HighlightStyle } = cmLang as any
  const { EditorView, keymap, lineNumbers, highlightActiveLine, drawSelection } = cmView as any
  const { defaultKeymap, indentWithTab } = cmCmds as any
  const { autocompletion } = cmAuto as any
  const { searchKeymap, highlightSelectionMatches, search } = cmSearch as any
  const { tags } = await import('@lezer/highlight')

  // CJS 互操作兜底：命名导出缺失回退 default；extension 用 flat 归一化（spread 非数组会抛错）
  const luaMode = legacy.lua ?? (legacy as any).default?.lua ?? (() => null)
  const baseKeymap = [
    indentWithTab,
    defaultKeymap ?? (cmCmds as any).default?.defaultKeymap,
    ...(searchKeymap ?? (cmSearch as any).default?.searchKeymap ?? []),
  ].flat(9).filter(Boolean)

  // token 配色
  const highlight = HighlightStyle.define([
    { tag: tags.keyword, color: '#c678dd' },
    { tag: tags.string, color: '#98c379' },
    { tag: tags.comment, color: '#7f848e', fontStyle: 'italic' },
    { tag: tags.number, color: '#d19a66' },
    { tag: tags.operator, color: '#56b6c2' },
    { tag: [tags.function(tags.variableName), tags.function(tags.propertyName)], color: '#61afef' },
    { tag: tags.propertyName, color: '#e06c75' },
    { tag: tags.variableName, color: '#e5c07b' },
  ])

  const completions = buildCompletionSource()
  view = new EditorView({
    parent: cmRef.value!,
    doc: props.modelValue,
    extensions: [
      lineNumbers(),
      highlightActiveLine(),
      drawSelection(),
      StreamLanguage.define(luaMode),
      syntaxHighlighting(highlight),
      autocompletion({ override: [completions], activateOnTyping: true }),
      ...(highlightSelectionMatches ? [highlightSelectionMatches()] : []),
      ...(search ? [search({ top: true })] : []),
      keymap.of(baseKeymap),
      EditorView.updateListener.of((u: any) => {
        if (u.docChanged && !syncFromCM) {
          emit('update:modelValue', u.state.doc.toString())
        }
      }),
    ],
  })
  ready.value = true
}

// 外部值变更同步进 CM
watch(
  () => props.modelValue,
  (v) => {
    if (!view) return
    const cur = view.state.doc.toString()
    if (v !== cur) {
      syncFromCM = true
      view.dispatch({ changes: { from: 0, to: cur.length, insert: v } })
      syncFromCM = false
    }
  },
)

function onInput(e: Event) {
  emit('update:modelValue', (e.target as HTMLTextAreaElement).value)
}

// 回退模式滚动联动行号槽
function onScroll(e: Event) {
  const ta = e.target as HTMLTextAreaElement
  if (gutterRef.value) {
    gutterRef.value.scrollTop = ta.scrollTop
  }
}

onMounted(() => {
  mountCM().catch((e) => { console.error('[CodeEditor] CodeMirror 懒加载失败，回退 textarea:', e) })
})
onBeforeUnmount(() => {
  view?.destroy()
  view = null
})
</script>

<style scoped>
.code-editor { display: flex; border: 1px solid var(--td-component-stroke); border-radius: 6px; overflow: hidden; background: #1e222a; }
.ce-cm { flex: 1; min-width: 0; }
.ce-cm :deep(.cm-editor) { height: 100%; font-size: 13px; }
.ce-cm :deep(.cm-editor) .cm-gutters { background: #1e222a; color: #5c6370; border-right: 1px solid #2c313a; }
.ce-cm :deep(.cm-content) { color: #d7dae0; }
.ce-cm :deep(.cm-activeLine) { background: #262b35; }
.ce-cm :deep(.cm-scroller) { font-family: ui-monospace, monospace; line-height: 20px; }
.ce-gutter {
  width: 44px; flex: none; padding: 8px 0; text-align: right; overflow: hidden;
  background: #1e222a; border-right: 1px solid #2c313a;
  user-select: none;
}
.ce-ln { padding-right: 8px; font-family: ui-monospace, monospace; font-size: 12px; line-height: 20px; color: #5c6370; }
.ce-ta {
  flex: 1; padding: 8px 10px; border: none; outline: none; resize: none;
  font-family: ui-monospace, monospace; font-size: 13px; line-height: 20px;
  white-space: pre; overflow: auto; background: transparent; color: #d7dae0;
}
</style>
