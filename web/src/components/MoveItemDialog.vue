<script setup>
// 物品移动：跨房间选择目标容器，改归属。支持"季节活动区换季收纳"场景。
import { ref, computed } from 'vue'
import { store, toast } from '../store'
import { TYPE_META, typeLabel, typeColor } from '../types'
import { api } from '../api'

const props = defineProps({ item: { type: Object, required: true } })
const emit = defineEmits(['moved', 'close'])

const roomId = ref(null)
const roomNodes = ref([])      // 当前房间全部容器
const loadErr = ref('')
const targetId = ref(null)
const moving = ref(false)

// 按 parent_id 递归构树，缩进扁平列表。
const tree = computed(() => {
  const byPar = {}
  roomNodes.value.forEach(l => {
    const k = l.parent_id ?? 0
    ;(byPar[k] ||= []).push(l)
  })
  const out = []
  const walk = (pid, depth) => {
    ;(byPar[pid] || []).forEach(l => { out.push({ l, depth }); walk(l.id, depth + 1) })
  }
  walk(0, 0)
  return out
})

// 整房间容器树（含房间内所有层级）——选择是跨房间的，加载其它房间的容器。
async function pickRoom(id) {
  roomId.value = id
  targetId.value = null
  loadErr.value = ''
  try {
    roomNodes.value = await api.roomLocations(id)
  } catch (e) { loadErr.value = e.message }
}

const target = computed(() => roomNodes.value.find(l => l.id === targetId.value) || null)
const targetItems = ref([])

async function onPick(l) {
  if (!canStore(l)) return   // 禁存物品的类型不可选
  targetId.value = l.id
  try {
    targetItems.value = await api.locationItems(l.id)
  } catch { targetItems.value = [] }
}

function canStore(l) { return !!(l && TYPE_META[l.location_type]?.allowItem) }

async function doMove() {
  if (!target.value || moving.value) return
  if (target.value.id === props.item.location_id) { toast('目标已是当前所在容器', 'info'); emit('close'); return }
  moving.value = true
  try {
    const it = props.item
    await api.updateItem(it.id, {
      location_id: target.value.id,
      name: it.name, qty: it.qty, remark: it.remark || '',
      stock_group: it.stock_group || '', min_qty: it.min_qty || 0,
      image_url: it.image_url || '',
      owner_id: it.owner_id || 0, is_private: it.is_private || 0,
      item_w: it.item_w || 0, item_d: it.item_d || 0, item_h: it.item_h || 0,
      category: it.category || '', tags: it.tags || '',
    })
    toast(`已移动「${it.name}」到 ${target.value.name}`, 'ok')
    emit('moved')
    emit('close')
  } catch (e) { toast(e.message, 'err') } finally { moving.value = false }
}
</script>

<template>
  <teleport to="body">
    <div class="mask" @click.self="emit('close')">
      <div class="dialog panel">
        <div class="row" style="align-items:baseline">
          <h3>移动「{{ item.name }}」</h3>
          <span class="dim" style="flex:1">选择目标容器</span>
          <button class="small" @click="emit('close')">取消</button>
        </div>

        <div class="cols">
          <!-- 左：房间列表 -->
          <div class="rooms">
            <div class="lbl">房间</div>
            <div
              v-for="r in store.rooms" :key="r.id"
              class="room" :class="{ on: r.id === roomId }" @click="pickRoom(r.id)">
              {{ r.name }}
            </div>
          </div>
          <!-- 右：容器树 -->
          <div class="locs">
            <div v-if="!roomId" class="dim">请先选房间</div>
            <div v-else-if="loadErr" class="err">{{ loadErr }}</div>
            <template v-else>
              <div
                v-for="{ l, depth } in tree" :key="l.id"
                class="loc" :class="{ disabled: !canStore(l), on: l.id === targetId }"
                :style="{ paddingLeft: (8 + depth * 16) + 'px' }"
                :title="canStore(l) ? (store.locations.find(x => x.id === l.id)?.name || '') : '该类型不能存放物品'"
                @click="onPick(l)">
                <span class="type-dot" :style="{ background: typeColor(l.location_type) }"></span>
                <span v-if="l.parent_id" class="dim" style="font-size:11px">└</span>
                <span class="name">{{ l.name }}</span>
                <span class="badge t">{{ typeLabel(l.location_type) }}</span>
                <span v-if="canStore(l)" class="dim cnt">{{ targetId === l.id ? `${targetItems.length} 件物品` : '' }}</span>
              </div>
            </template>
          </div>
        </div>

        <div class="foot row">
          <span class="dim" v-if="target">目标：{{ target.name }}（现存 {{ targetItems.length }} 件{{ canStore(target) ? '' : '，不可存放，已禁用' }}）</span>
          <span style="flex:1"></span>
          <button class="small primary" :disabled="!target || moving" @click="doMove">
            {{ moving ? '移动中…' : '移动到此' }}
          </button>
        </div>
      </div>
    </div>
  </teleport>
</template>

<style scoped>
.mask {
  position: fixed; inset: 0; background: rgba(40, 50, 70, .4);
  display: flex; align-items: center; justify-content: center; z-index: 100;
}
.dialog { padding: 16px; width: min(760px, 96vw); height: min(70vh, 560px); display: flex; flex-direction: column; gap: 12px; }
.dialog h3 { margin: 0; }
.cols { display: flex; flex: 1; min-height: 0; gap: 10px; }
.rooms { width: 150px; border-right: 1px solid var(--border); overflow: auto; flex-shrink: 0; }
.locs { flex: 1; overflow: auto; }
.lbl { font-size: 12px; color: var(--fg-dim); margin-bottom: 6px; }
.room { padding: 7px 8px; cursor: pointer; border-radius: 6px; margin-bottom: 2px; }
.room:hover { background: var(--bg3); }
.room.on { background: var(--accent-weak); font-weight: 600; }
.loc { padding: 6px 8px; cursor: pointer; border-radius: 6px; display: flex; gap: 6px; align-items: center; margin-bottom: 1px; }
.loc:hover { background: var(--bg3); }
.loc.disabled { opacity: .5; cursor: not-allowed; }
.loc.on { background: var(--accent-weak); outline: 1px solid var(--accent); }
.loc .name { font-weight: 500; }
.cnt { font-size: 11px; }
.err { color: var(--danger); padding: 8px; }
.foot { border-top: 1px solid var(--border); padding-top: 10px; }
</style>