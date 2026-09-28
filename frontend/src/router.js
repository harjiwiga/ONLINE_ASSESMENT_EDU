import { createRouter, createWebHistory } from 'vue-router'
import ParentBooking from './views/ParentBooking.vue'
import BookingResult from './views/BookingResult.vue'
import AdminRoster from './views/AdminRoster.vue'

export default createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: ParentBooking },
    { path: '/bookings/:id', component: BookingResult },
    { path: '/admin', component: AdminRoster },
  ],
})
