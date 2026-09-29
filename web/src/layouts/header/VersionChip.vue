<!-- VersionChip — 版本徽标：有更新高亮 + 红点，点击查更新日志。 -->
<template>
  <t-tooltip v-if="version" :content="updateAvailable ? $t('common.hasUpdate') : $t('common.checkUpdate')">
    <div class="ver-chip" :class="{ 'has-update': updateAvailable, checking }" @click="checkVersion(true)">
      <t-loading v-if="checking" size="12px" />
      <span v-else class="ver-text">v{{ version }}</span>
      <span v-if="updateAvailable && !checking" class="ver-dot" />
    </div>
  </t-tooltip>

  <!-- 更新日志弹窗（CDialog：移动端 85% 宽居中，内容限高滚动） -->
  <c-dialog v-model:visible="changelogVisible" :header="changelogTitle" :footer="false" width="480px">
    <div class="changelog-ver">v{{ version }} → <b>v{{ latest }}</b></div>
    <ul v-if="changelog?.items?.length" class="changelog-list">
      <li v-for="(it, i) in changelog.items" :key="i">{{ clText(it) }}</li>
    </ul>
    <div class="changelog-foot">
      <t-button theme="primary" @click="gotoRelease">{{ $t('common.goRelease') }}</t-button>
    </div>
  </c-dialog>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { CDialog } from '@/components/base'
import { versionApi, type Changelog } from '@/api/auth'
import { useLocalizedText } from '@/composables'

const { t } = useI18n()
const clText = useLocalizedText()

const version = ref('')
const latest = ref('')
const updateAvailable = ref(false)
const checking = ref(false)
const releaseUrl = ref('https://github.com/ShadowSmallBaby/ClawProxyHub/releases')
const changelog = ref<Changelog | null>(null)
const changelogVisible = ref(false)

async function checkVersion(manual = false) {
  if (checking.value) return
  checking.value = true
  try {
    const r = await versionApi.get()
    version.value = r.version
    latest.value = r.latest ?? ''
    updateAvailable.value = !!r.update_available
    if (r.release_url) releaseUrl.value = r.release_url
    changelog.value = r.changelog ?? null
    if (manual) {
      if (updateAvailable.value) {
        changelogVisible.value = true
      } else if (latest.value) {
        MessagePlugin.success(t('common.upToDate'))
      } else {
        MessagePlugin.warning(t('common.checkFailed'))
      }
    }
  } catch {
    if (manual) MessagePlugin.warning(t('common.checkFailed'))
  } finally {
    checking.value = false
  }
}

const changelogTitle = computed(() => clText(changelog.value?.title) || t('common.hasUpdate'))

function gotoRelease() {
  window.open(releaseUrl.value, '_blank')
  changelogVisible.value = false
}

onMounted(() => checkVersion()) // 首次进页自动查一次
</script>

<style scoped>
/* 版本 chip：默认灰字，有更新时描边高亮 + 红点，可点跳发布页 */
.ver-chip {
  display: flex;
  align-items: center;
  gap: 5px;
  height: 26px;
  padding: 0 10px;
  border-radius: 13px;
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  color: var(--td-text-color-placeholder);
  cursor: pointer;
  transition: color 0.2s ease-out, background-color 0.2s ease-out;
}
.ver-chip:hover {
  color: var(--td-text-color-primary);
  background: var(--td-bg-color-secondarycontainer);
}
.ver-chip.checking {
  cursor: progress;
}
.ver-chip.has-update {
  color: var(--td-warning-color);
  background: var(--td-warning-color-1);
  cursor: pointer;
}
.ver-chip.has-update:hover {
  background: var(--td-warning-color-2);
}
.ver-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--td-warning-color);
}

/* 更新日志弹窗 */
.changelog-ver {
  font-size: 13px;
  color: var(--td-text-color-secondary);
  margin-bottom: 12px;
  font-variant-numeric: tabular-nums;
}
.changelog-ver b {
  color: var(--td-brand-color);
}
.changelog-list {
  margin: 0;
  padding-left: 20px;
  line-height: 1.9;
  font-size: 14px;
  color: var(--td-text-color-primary);
  text-align: justify;
  text-align-last: left;
}
.changelog-foot {
  display: flex;
  justify-content: flex-end;
  margin-top: 20px;
}
/* 手机端：内容限高滚动，间距收窄 */
@media (max-width: 767px) {
  .changelog-ver {
    margin-bottom: 8px;
  }
  .changelog-list {
    max-height: 50vh;
    overflow-y: auto;
    line-height: 1.7;
  }
  .changelog-foot {
    margin-top: 12px;
  }
}
</style>
