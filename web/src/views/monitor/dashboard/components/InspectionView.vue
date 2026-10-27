<script setup>
import InspectionOverview from './inspection/InspectionOverview.vue'
import InspectionResultArea from './inspection/InspectionResultArea.vue'
import { useDashboardDomain } from '../context'
const { isFullscreen } = useDashboardDomain('fullscreen')
</script>

<template>
  <InspectionOverview v-if="!isFullscreen" />
  <InspectionResultArea />
</template>
