<script setup>
import { computed } from 'vue'
import { RouterLink, RouterView, useRoute } from 'vue-router'
import { PARENT_LIST, getParentId, setParentId } from './api.js'

const route = useRoute()
const showParent = computed(() => !route.path.startsWith('/admin'))

const parentId = computed({
  get: () => getParentId(),
  set: (v) => {
    setParentId(v)
    window.location.reload()
  },
})
</script>

<template>
  <div class="app-shell">
    <header class="topbar">
      <RouterLink to="/" class="brand">
        <div class="mark">O</div>
        <div class="brand-text">
          <small>Ottodot</small>
          Trials
        </div>
      </RouterLink>
      <nav class="nav-links">
        <RouterLink to="/">Classes</RouterLink>
        <RouterLink to="/admin">Roster</RouterLink>
      </nav>
      <label v-if="showParent" class="parent-switch">
        Parent
        <select v-model="parentId" aria-label="Parent account">
          <option v-for="p in PARENT_LIST" :key="p.id" :value="p.id">{{ p.name }}</option>
        </select>
      </label>
      <span v-else class="parent-spacer" aria-hidden="true"></span>
    </header>
    <RouterView />
  </div>
</template>
