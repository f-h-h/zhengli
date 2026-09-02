<script setup>
// 回收站：软删记录的浏览、恢复、彻底删除、清空。
import { ref, computed } from 'vue'
import { store, loadRooms, selectRoom, reloadLocations, toast } from '../store'
import { api } from '../api'

const open = ref(false)
const entries = ref([])
const loading = ref(false)

const KIND_LABEL = { room: '房间', location: '容器', item: '物品' }
const TYPE_LABEL = {
  cabinet: '衣柜', drawer: '抽屉', shelf_layer: '层板', box: '收纳箱',
  cardbox: '纸箱', luggage: '行李箱', wall_mount: '挂墙收纳', table: '桌面',
  hanger: '挂衣杆', tray: '托盘', ground_spot: '地面区域', furniture: '家具',
  obstacle: '障碍物', door: '门', window: '窗户',
}

async function show() {
  open.value = true
  await refresh()
}

async function refresh() {
  loading.value = true
  try {
    entries.value = await api.listTrash()
  } catch (e) { toast(e.message, 'err') }
  loading.value = false
}

const groups = computed(() => {
  const g = { room: [], location: [], item: [] }
  for (const e of entries.value) g[e.kind]?.push(e)
  return g
})

async function restore(e) {
  try {
    await api.restoreTrash(e.kind, e.id)
    toast(`已恢复「${e.name}」`, 'ok')
    await refresh()
    // 恢复可能带回了房间/容器：全量刷新保持树与房间列表一致。
    await loadRooms(true)
    if (store.currentRoomId) await reloadLocations()
  } catch (err) { toast(err.message, 'err') }
}

async function purge(e) {
  if (!confirm(`彻底删除「${e.name}」？此操作不可恢复${e.kind === 'location' ? '，其下级容器与物品将一并删除' : ''}。`)) return
  try {
    await api.purgeTrash(e.kind, e.id)
    toast('已彻底删除', 'ok')
    await refresh()
  } catch (err) { toast(err.message, 'err') }
}

async function purgeAll() {
  if (!entries.value.length) return
  if (!confirm(`清空回收站（${entries.value.length} 条）？此操作不可恢复。`)) return
  try {
    await api.emptyTrash()
    toast('回收站已清空', 'ok')
    entries.value = []
  } catch (err) { toast(err.message, 'err') }
}

function desc(e) {
  const parts = []
  if (e.kind === 'location') {
    parts.push(TYPE_LABEL[e.type] || e.type)
    if (e.parent_name) parts.push(`在 ${e.parent_name}`)
  } else if (e.kind === 'item') {
    if (e.qty > 1) parts.push(`×${e.qty}`)
    if (e.parent_name) parts.push(`在 ${e.parent_name}`)
  }
  if (e.room_name) parts.push(`· ${e.room_name}`)
  return parts.join(' ')
}

defineExpose({ show })
</script>

<template>
  <button class="small" @click="show">🗑 回收站</button>

  <teleport to="body">
    <div v-if="open" class="mask" @click.self="open = false">
      <div class="dialog panel">
        <div class="row" style="align-items:baseline">
          <h3>回收站（{{ entries.length }}）</h3>
          <span class="dim" style="flex:1">删除的记录可恢复；彻底删除不可撤销</span>
          <button v-if="entries.length" class="small danger" @click="purgeAll">清空</button>
          <button class="small" @click="open = false">关闭</button>
        </div>

        <div v-if="loading" class="dim" style="padding:20px;text-align:center">加载中…</div>
        <div v-else-if="!entries.length" class="dim" style="padding:20px;text-align:center">回收站是空的</div>

        <div v-else class="list">
          <template v-for="kind in ['room','location','item']" :key="kind">
            <div v-if="groups[kind].length" class="group-h">{{ KIND_LABEL[kind] }}（{{ groups[kind].length }}）</div>
            <div v-for="e in groups[kind]" :key="kind + e.id" class="entry row">
              <div class="grow">
                <div class="name">{{ e.name }}</div>
                <div class="dim sub">{{ desc(e) }} · {{ e.deleted_at.replace('T', ' ') }}</div>
              </div>
              <button class="small primary" @click="restore(e)">↩ 恢复</button>
              <button class="small danger" @click="purge(e)">彻底删除</button>
            </div>
          </template>
        </div>
      </div>
    </div>
  </teleport>
</template>

<style scoped>
.mask {
  position: fixed; inset: 0; background: rgba(40, 50, 70, .38);
  display: flex; align-items: center; justify-content: center; z-index: 100;
}
.dialog { padding: 16px; width: min(560px, 94vw); max-height: 82vh; display: flex; flex-direction: column; }
.dialog h3 { margin-bottom: 10px; }
.list { overflow: auto; }
.group-h {
  font-size: 12px; color: var(--fg-dim); margin: 10px 0 4px;
  border-bottom: 1px solid var(--border); padding-bottom: 3px;
}
.entry { padding: 6px 0; gap: 8px; align-items: center; }
.entry .name { font-weight: 500; }
.sub { font-size: 12px; }
.danger { color: #c0392b; border-color: #e5b4ae; }
.danger:hover { background: #fdf0ee; }
</style>
