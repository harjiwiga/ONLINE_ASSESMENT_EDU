const PARENTS = [
  { id: 'par_maya', name: 'Maya' },
  { id: 'par_ben', name: 'Ben' },
  { id: 'par_chen', name: 'Chen' },
  { id: 'par_dana', name: 'Dana' },
  { id: 'par_priya', name: 'Priya' },
  { id: 'par_omar', name: 'Omar' },
]

export function getParentId() {
  return localStorage.getItem('parentId') || 'par_maya'
}

export function setParentId(id) {
  localStorage.setItem('parentId', id)
}

export const PARENT_LIST = PARENTS

export function money(cents) {
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format((cents || 0) / 100)
}

export function classWhen(iso) {
  if (!iso) return ''
  const d = new Date(iso)
  const day = d.toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric' })
  const time = d.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })
  return `${day} · ${time}`
}

export function prettyStatus(status) {
  return (
    {
      pending_payment: 'Reserved',
      confirmed: 'Confirmed',
      expired: 'Expired',
      cancelled: 'Cancelled',
      seat_unavailable: 'Not seated',
    }[status] || status
  )
}

async function request(path, { method = 'GET', body, parentId, admin } = {}) {
  const headers = { Accept: 'application/json' }
  if (body) headers['Content-Type'] = 'application/json'
  if (admin) headers['X-Admin-Role'] = 'teacher'
  else headers['X-Parent-Id'] = parentId || getParentId()
  const res = await fetch(path, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  })
  const text = await res.text()
  let data = null
  try {
    data = text ? JSON.parse(text) : null
  } catch {
    data = { message: text }
  }
  if (!res.ok) {
    const err = new Error(data?.message || res.statusText)
    err.code = data?.code
    err.status = res.status
    err.body = data
    throw err
  }
  return data
}

export const api = {
  students: () => request('/api/me/students'),
  classes: () => request('/api/trial-classes'),
  createBooking: (studentId, trialClassId) =>
    request('/api/bookings', { method: 'POST', body: { studentId, trialClassId } }),
  getBooking: (id) => request(`/api/bookings/${id}`),
  pay: (id, outcome) =>
    request(`/api/bookings/${id}/pay`, {
      method: 'POST',
      body: { idempotencyKey: `pay_${id}_${Date.now()}_${outcome}`, outcome },
    }),
  cancel: (id) => request(`/api/bookings/${id}/cancel`, { method: 'POST', body: {} }),
  roster: (classId) => request(`/api/admin/trial-classes/${classId}/roster`, { admin: true }),
}
