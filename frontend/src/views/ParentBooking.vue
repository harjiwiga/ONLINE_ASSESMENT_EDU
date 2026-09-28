<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api, classWhen, money } from '../api.js'

const router = useRouter()
const students = ref([])
const classes = ref([])
const studentId = ref('')
const trialClassId = ref('')
const error = ref('')
const busy = ref(false)
const loading = ref(true)

const selectedClass = computed(() => classes.value.find((c) => c.id === trialClassId.value))
const selectedStudent = computed(() => students.value.find((s) => s.id === studentId.value))
const canHold = computed(
  () => Boolean(studentId.value && trialClassId.value && selectedClass.value?.remainingSeats > 0 && !busy.value),
)

onMounted(async () => {
  try {
    students.value = await api.students()
    classes.value = await api.classes()
    if (students.value[0]) studentId.value = students.value[0].id
    const open = classes.value.find((c) => c.remainingSeats > 0)
    if (open) trialClassId.value = open.id
    else if (classes.value[0]) trialClassId.value = classes.value[0].id
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
})

function seatKinds(c) {
  const dots = []
  for (let i = 0; i < c.confirmedCount; i++) dots.push('confirmed')
  for (let i = 0; i < c.heldCount; i++) dots.push('held')
  for (let i = 0; i < c.remainingSeats; i++) dots.push('free')
  return dots.slice(0, c.capacity || 4)
}

function seatsLeft(c) {
  if (c.remainingSeats === 0) return 'No seats left'
  if (c.remainingSeats === 1) return '1 seat left'
  return `${c.remainingSeats} seats left`
}

function selectClass(id) {
  trialClassId.value = id
}

async function submit() {
  error.value = ''
  busy.value = true
  try {
    const b = await api.createBooking(studentId.value, trialClassId.value)
    router.push(`/bookings/${b.id}`)
  } catch (e) {
    error.value = e.message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <section class="hero">
    <h1>Book a trial class</h1>
    <p class="lede">Pick your child, then a class. We’ll hold a seat for 8 minutes while you pay.</p>
  </section>

  <div v-if="error" class="alert">{{ error }}</div>
  <p v-if="loading" class="loading">Loading classes…</p>

  <template v-else>
    <div class="section-head">
      <p class="section-label"><span class="step">1</span> Who is this for?</p>
    </div>
    <p v-if="!students.length" class="empty-note">No children on this account.</p>
    <div v-else class="chip-row">
      <button
        v-for="s in students"
        :key="s.id"
        type="button"
        class="chip"
        :class="{ active: studentId === s.id }"
        :aria-pressed="studentId === s.id"
        @click="studentId = s.id"
      >
        <b>{{ s.name }}</b>
        <small>{{ s.age ? `Age ${s.age}` : 'Student' }}</small>
      </button>
    </div>

    <div class="section-head">
      <p class="section-label"><span class="step">2</span> Choose a class</p>
    </div>
    <div class="class-grid">
      <article
        v-for="c in classes"
        :key="c.id"
        class="class-card"
        :class="{ active: trialClassId === c.id, full: c.remainingSeats === 0 }"
        role="button"
        tabindex="0"
        :aria-pressed="trialClassId === c.id"
        @click="selectClass(c.id)"
        @keydown.enter.prevent="selectClass(c.id)"
        @keydown.space.prevent="selectClass(c.id)"
      >
        <div class="card-top">
          <span class="badge" :class="c.subject">{{ c.subject }}</span>
          <span v-if="c.remainingSeats === 0" class="badge full">Full</span>
          <span v-else-if="c.remainingSeats === 1" class="badge warn">Last seat</span>
        </div>
        <h2>{{ c.title }}</h2>
        <p class="meta">{{ classWhen(c.startsAt) }}</p>
        <p class="meta">{{ c.teacherName }}</p>
        <div class="seat-dots" :aria-label="seatsLeft(c)">
          <span v-for="(kind, i) in seatKinds(c)" :key="i" class="dot" :class="kind"></span>
        </div>
        <div class="card-foot">
          <span class="price">{{ money(c.amountCents) }}</span>
          <span class="meta">{{ seatsLeft(c) }}</span>
        </div>
        <div v-if="trialClassId === c.id" class="card-cta" @click.stop>
          <button class="btn btn-primary" :disabled="!canHold" @click="submit">
            {{ busy ? 'Reserving…' : c.remainingSeats === 0 ? 'Class is full' : `Reserve for ${selectedStudent?.name || 'your child'}` }}
          </button>
        </div>
      </article>
    </div>
  </template>
</template>
