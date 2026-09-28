<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { api, classWhen } from '../api.js'

const classes = ref([])
const classId = ref('')
const roster = ref(null)
const error = ref('')

const selected = computed(() => classes.value.find((c) => c.id === classId.value) || roster.value)
const confirmedPct = computed(() => pct(roster.value?.confirmedCount))
const heldPct = computed(() => pct(roster.value?.heldCount))

function pct(n) {
  const cap = roster.value?.capacity || selected.value?.capacity || 4
  return `${((n || 0) / cap) * 100}%`
}

async function loadClasses() {
  classes.value = await api.classes()
  if (!classId.value && classes.value[0]) classId.value = classes.value[0].id
}

async function loadRoster() {
  if (!classId.value) return
  error.value = ''
  roster.value = await api.roster(classId.value)
}

onMounted(async () => {
  try {
    await loadClasses()
    await loadRoster()
  } catch (e) {
    error.value = e.message
  }
})

watch(classId, () => {
  loadRoster().catch((e) => {
    error.value = e.message
  })
})
</script>

<template>
  <section class="hero">
    <h1>Class roster</h1>
    <p class="lede">Only students who have paid and been seated appear here.</p>
  </section>

  <div v-if="error" class="alert">{{ error }}</div>

  <div class="chip-row roster-classes">
    <button
      v-for="c in classes"
      :key="c.id"
      type="button"
      class="chip"
      :class="{ active: classId === c.id }"
      @click="classId = c.id"
    >
      <b>{{ c.title }}</b>
      <small>{{ c.remainingSeats === 0 ? 'Full' : `${c.remainingSeats} open` }}</small>
    </button>
  </div>

  <div v-if="roster">
    <div class="stats">
      <div class="stat"><b>{{ roster.confirmedCount }}</b><span>Confirmed</span></div>
      <div class="stat"><b>{{ roster.heldCount }}</b><span>Waiting to pay</span></div>
      <div class="stat"><b>{{ roster.remainingSeats }}</b><span>Open</span></div>
    </div>
    <div class="capacity" aria-hidden="true">
      <i class="confirmed" :style="{ width: confirmedPct }"></i>
      <i class="held" :style="{ width: heldPct }"></i>
    </div>
    <div class="table-wrap">
      <table>
        <thead>
          <tr>
            <th>Student</th>
            <th>Parent</th>
            <th>Confirmed</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="!roster.students.length">
            <td class="empty" colspan="3">No confirmed students yet.</td>
          </tr>
          <tr v-for="s in roster.students" :key="s.bookingId">
            <td><b>{{ s.studentName }}</b></td>
            <td>{{ s.parentName }}</td>
            <td>{{ classWhen(s.confirmedAt) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
