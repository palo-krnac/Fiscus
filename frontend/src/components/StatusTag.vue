<script setup>
import { computed } from 'vue'
import { statusOf } from '../lib/format.js'

const props = defineProps({ row: { type: Object, required: true } })
const s = computed(() => statusOf(props.row))
</script>

<template>
  <span class="tag" :data-tone="s.tone"><span class="tag-text">{{ s.label }}</span></span>
</template>
