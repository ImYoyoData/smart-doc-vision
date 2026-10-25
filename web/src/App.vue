<script setup lang="ts">
import {
  NConfigProvider,
  NDialogProvider,
  NGlobalStyle,
  NLoadingBarProvider,
  NMessageProvider,
  darkTheme,
  zhCN,
  dateZhCN,
} from 'naive-ui'
import { darkThemeOverrides, themeOverrides } from './theme'
import { useThemeStore } from '@/stores/theme'

const theme = useThemeStore()
theme.init()
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
          <router-view />
        </n-dialog-provider>
      </n-message-provider>
    </n-loading-bar-provider>
  </n-config-provider>
</template>
