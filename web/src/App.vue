<script setup lang="ts">
import {
  NConfigProvider,
  NDialogProvider,
  NGlobalStyle,
  NLoadingBarProvider,
  NMessageProvider,
  NSpin,
  darkTheme,
  zhCN,
  dateZhCN,
} from 'naive-ui'
import { darkThemeOverrides, themeOverrides } from './theme'
import { useThemeStore } from '@/stores/theme'
import { useInstallStore } from '@/stores/install'
import InstallView from '@/views/InstallView.vue'

const theme = useThemeStore()
theme.init()
const install = useInstallStore()
void install.load()
</script>

<template>
  <n-config-provider
    :theme="theme.isDark ? darkTheme : null"
    :theme-overrides="theme.isDark ? darkThemeOverrides : themeOverrides"
    :locale="zhCN"
    :date-locale="dateZhCN"
  >
    <n-global-style />
    <n-loading-bar-provider>
      <n-message-provider placement="top-right" :max="4">
        <n-dialog-provider>
          <div v-if="!install.checked" class="boot-screen">
            <n-spin size="small" />
          </div>
          <InstallView v-else-if="install.isInstallerState()" />
          <router-view v-else />
        </n-dialog-provider>
      </n-message-provider>
    </n-loading-bar-provider>
  </n-config-provider>
</template>

<style scoped>
.boot-screen {
  display: flex;
  min-height: 100vh;
  align-items: center;
  justify-content: center;
  background: var(--canvas);
}
</style>
