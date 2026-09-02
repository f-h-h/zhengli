// 全局响应式状态：房间、当前房间 locations、选中项、提示消息。
import { reactive } from 'vue'
import { api } from './api'

export const store = reactive({
  rooms: [],
  currentRoomId: null,
  household: null,     // 当前家（数据归属顶层单位）
  locations: [],        // 当前房间全部 locations
  selectedId: null,    // 3D/树中选中的 location
  toasts: [],
  view: 'tree',        // 移动端视图：tree | search | scene
  isMobile: window.innerWidth < 768,
  flashTarget: null,   // 寻物定位：3D 中闪烁提示的 location id
})

// id → location 索引：3D 拖拽热路径与深度/面包屑 O(1) 查找（避免每帧 find）。
export const locById = new Map()

// 在 locations 变更后重建索引。
export function refreshIndex() {
  locById.clear()
  for (const l of store.locations) locById.set(l.id, l)
}

let toastSeq = 0

export function toast(msg, type = 'info') {
  const id = ++toastSeq
  store.toasts.push({ id, msg, type })
  setTimeout(() => {
    const i = store.toasts.findIndex(t => t.id === id)
    if (i >= 0) store.toasts.splice(i, 1)
  }, 3200)
}

export const selectedLoc = () =>
  locById.get(store.selectedId) ?? null

export const currentRoom = () =>
  store.rooms.find(r => r.id === store.currentRoomId) ?? null

// 业务归属深度：沿 parent_id 链上溯。
export function depthOf(loc) {
  let d = 0
  let cur = loc
  while (cur && cur.parent_id) {
    const p = locById.get(cur.parent_id)
    if (!p) break
    d++
    cur = p
  }
  return d
}

// 业务归属链（根→当前），面包屑展示用。
export function chainOf(loc) {
  const chain = []
  let cur = loc
  while (cur) {
    chain.unshift(cur)
    cur = cur.parent_id ? locById.get(cur.parent_id) : null
  }
  return chain
}

export async function loadRooms(selectFirst = false) {
  store.rooms = await api.listRooms()
  if (selectFirst && store.rooms.length && !store.currentRoomId) {
    await selectRoom(store.rooms[0].id)
  }
}

// 加载当前家信息（顶栏展示；单用户阶段固定默认家）。
export async function loadHousehold() {
  try {
    store.household = await api.currentHousehold()
  } catch {
    store.household = null
  }
}

export async function selectRoom(id) {
  store.currentRoomId = id
  store.selectedId = null
  await reloadLocations()
}

export async function reloadLocations() {
  if (!store.currentRoomId) { store.locations = []; refreshIndex(); return }
  store.locations = await api.roomLocations(store.currentRoomId)
  refreshIndex()
}

export function select(id) {
  store.selectedId = id
  if (id && store.isMobile) store.view = 'detail'
}
