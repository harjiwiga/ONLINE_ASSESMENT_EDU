<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { api, classWhen, money, prettyStatus } from '../api.js'

const route = useRoute()
const booking = ref(null)
const error = ref('')
const remaining = ref('')
const paying = ref(false)
let timer

const tone = computed(() => {
  const s = booking.value?.status
  if (s === 'confirmed') return 'ok'
  if (s === 'pending_payment') return 'warn'
  if (s === 'seat_unavailable' || s === 'expired') return 'bad'
  return 'neutral'
})

const title = computed(() => {
  const b = booking.value
  if (!b) return 'Booking'
  if (b.status === 'confirmed') return 'You’re booked'
  if (b.status === 'pending_payment' && b.lastPaymentResult === 'failure') return 'Payment declined'
  if (b.status === 'pending_payment') return 'Seat reserved'
  if (b.status === 'expired') return 'Reservation expired'
  if (b.status === 'cancelled') return 'Reservation cancelled'
  if (b.status === 'seat_unavailable') return 'This seat was taken'
  return 'Booking'
})

const copy = computed(() => {
  const b = booking.value
  if (!b) return ''
  if (b.status === 'confirmed') return `${b.studentName} is confirmed for ${b.classTitle}.`
  if (b.status === 'pending_payment' && b.lastPaymentResult === 'failure') {
    return `That card didn’t go through. ${b.studentName}’s seat is still reserved — try again before time runs out.`
  }
  if (b.status === 'pending_payment') {
    return `We’ve held a seat for ${b.studentName}. Pay now to confirm. You won’t be charged if the timer runs out.`
  }
  if (b.status === 'expired') return 'The 8-minute hold ended and the seat was released. Nobody was charged.'
  if (b.status === 'cancelled') return 'The seat is available again for other families.'
  if (b.status === 'seat_unavailable') {
    return 'Payment arrived after the hold ended, so this child is not on the roster. A refund will be recorded.'
  }
  return prettyStatus(b.status)
})

const canPay = computed(() => booking.value?.status === 'pending_payment' && remaining.value !== '00:00')

function tick() {
  const iso = booking.value?.holdExpiresAt
  if (!iso || booking.value?.status !== 'pending_payment') {
    remaining.value = ''
    return
  }
  const ms = new Date(iso).getTime() - Date.now()
  if (ms <= 0) remaining.value = '00:00'
  else remaining.value = `${String(Math.floor(ms / 60000)).padStart(2, '0')}:${String(Math.floor((ms / 1000) % 60)).padStart(2, '0')}`
}

async function load() {
  booking.value = await api.getBooking(route.params.id)
  tick()
}

async function pay(outcome) {
  error.value = ''
  paying.value = true
  try {
    booking.value = await api.pay(route.params.id, outcome)
    tick()
  } catch (e) {
    error.value = e.message
    try { await load() } catch { /* ignore */ }
  } finally {
    paying.value = false
  }
}

async function cancel() {
  error.value = ''
  try {
    booking.value = await api.cancel(route.params.id)
  } catch (e) {
    error.value = e.message
  }
}

onMounted(async () => {
  try {
    await load()
    timer = setInterval(tick, 1000)
  } catch (e) {
    error.value = e.message
  }
})
onUnmounted(() => timer && clearInterval(timer))
</script>

<template>
  <p class="back"><RouterLink to="/">← Classes</RouterLink></p>
  <section class="hero">
    <h1>{{ title }}</h1>
    <p class="lede">{{ copy }}</p>
  </section>
  <div v-if="error" class="alert">{{ error }}</div>

  <div v-if="booking" class="checkout">
    <div v-if="canPay" class="hold-bar">
      <div>
        <small>Time left to pay</small>
        <strong>{{ remaining || '—' }}</strong>
      </div>
      <p>No charge if this runs out</p>
    </div>
    <div v-else class="hold-bar" :class="tone">
      <div>
        <small>Status</small>
        <strong>{{ prettyStatus(booking.status) }}</strong>
      </div>
    </div>

    <div class="checkout-class">
      <div>
        <span class="badge" :class="booking.classSubject">{{ booking.classSubject }}</span>
        <h2>{{ booking.classTitle }}</h2>
        <p class="meta">{{ classWhen(booking.classStartsAt) }} · {{ booking.teacherName }}</p>
      </div>
      <div class="price">{{ money(booking.amountCents) }}</div>
    </div>

    <dl class="facts">
      <div><dt>Child</dt><dd>{{ booking.studentName }}</dd></div>
      <div><dt>Parent</dt><dd>{{ booking.parentName }}</dd></div>
    </dl>

    <p v-if="booking.refundNeeded" class="alert">Refund needed — this child is not on the roster.</p>

    <div v-if="canPay" class="checkout-actions">
      <button class="btn btn-primary" :disabled="paying" @click="pay('success')">
        {{ paying ? 'Processing…' : `Pay ${money(booking.amountCents)}` }}
      </button>
      <div class="quiet-actions">
        <button type="button" class="text-btn" :disabled="paying" @click="pay('failure')">Try declined card</button>
        <button type="button" class="text-btn" :disabled="paying" @click="cancel">Cancel reservation</button>
      </div>
    </div>
    <div v-else class="checkout-actions">
      <RouterLink class="btn btn-secondary" to="/">Back to classes</RouterLink>
    </div>
  </div>
</template>
