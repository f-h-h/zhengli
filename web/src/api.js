// REST API 封装：统一错误提取。
async function req(method, url, body, raw = false) {
  const opts = { method }
  if (body !== undefined) {
    if (raw) {
      opts.body = body
    } else {
      opts.headers = { 'Content-Type': 'application/json' }
      opts.body = JSON.stringify(body)
    }
  }
  const res = await fetch(url, opts)
  if (res.status === 204) return null
  const text = await res.text()
  const data = raw ? text : safeJSON(text)
  if (!res.ok) {
    throw new Error((data && data.error) ? data.error : `HTTP ${res.status}`)
  }
  return data
}

function safeJSON(text) {
  try { return JSON.parse(text) } catch { return { error: text } }
}

export const api = {
  // households（家：数据归属顶层单位）
  listHouseholds: () => req('GET', '/api/households'),
  currentHousehold: () => req('GET', '/api/households/current'),
  listMembers: () => req('GET', '/api/households/members'),

  // rooms
  listRooms: () => req('GET', '/api/rooms'),
  createRoom: (room) => req('POST', '/api/rooms', room),
  updateRoom: (id, room) => req('PUT', `/api/rooms/${id}`, room),
  deleteRoom: (id) => req('DELETE', `/api/rooms/${id}`),
  roomLocations: (roomId) => req('GET', `/api/rooms/${roomId}/locations`),

  // locations
  createLocation: (l) => req('POST', '/api/locations', l),
  getLocation: (id) => req('GET', `/api/locations/${id}`),
  updateLocation: (id, l) => req('PUT', `/api/locations/${id}`, l),
  patchGeom: (id, g) => req('PATCH', `/api/locations/${id}/geom`, g),
  deleteLocation: (id) => req('DELETE', `/api/locations/${id}`),
  children: (id) => req('GET', `/api/locations/${id}/children`),
  descendants: (id) => req('GET', `/api/locations/${id}/descendants`),
  breadcrumb: (id) => req('GET', `/api/locations/${id}/breadcrumb`),
  locationItems: (id) => req('GET', `/api/locations/${id}/items`),
  treeItems: (id) => req('GET', `/api/locations/${id}/tree-items`),

  // items
  createItem: (it) => req('POST', '/api/items', it),
  updateItem: (id, it) => req('PUT', `/api/items/${id}`, it),
  deleteItem: (id) => req('DELETE', `/api/items/${id}`),
  searchItems: (q) => req('GET', `/api/items/search?q=${encodeURIComponent(q)}`),
  uploadItemImage: async (id, file) => {
    const fd = new FormData()
    fd.append('file', file)
    const res = await fetch(`/api/items/${id}/image`, { method: 'POST', body: fd })
    const data = safeJSON(await res.text())
    if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`)
    return data
  },
  deleteItemImage: (id) => req('DELETE', `/api/items/${id}/image`),

  // import / export
  importLocations: (roomId, csvText, mode) =>
    req('POST', `/api/rooms/${roomId}/import/locations?mode=${mode}`, csvText, true),
  importItems: (csvText) => req('POST', '/api/items/import', csvText, true),

  // 回收站（软删除）
  listTrash: () => req('GET', '/api/trash'),
  restoreTrash: (kind, id) => req('POST', `/api/trash/${kind}/${id}/restore`),
  purgeTrash: (kind, id) => req('DELETE', `/api/trash/${kind}/${id}`),
  emptyTrash: () => req('DELETE', '/api/trash'),

  // 库存组（在用/补充装）
  consumeItem: (id, fromId) =>
    req('POST', `/api/items/${id}/consume`, fromId ? { from_item_id: fromId } : {}),
  restockItem: (id, qty) => req('POST', `/api/items/${id}/restock`, { qty }),
  markEmpty: (id) => req('POST', `/api/items/${id}/empty`),
  shoppingList: () => req('GET', '/api/shopping-list'),
}

// 导出下载（浏览器直接访问 GET 链接）。
export function downloadExport(roomId, kind) {
  window.open(`/api/rooms/${roomId}/export/${kind}`, '_blank')
}
