<script setup>
// 搜索面板（移动端主入口）：搜物品 → 面包屑定位。
import { ref, computed, nextTick } from 'vue'
import { store, toast, select, reloadLocations } from '../store'
import { api } from '../api'
import { typeLabel, typeColor } from '../types'

const q = ref('')
const results = ref([])    // items
const locMap = ref({})     // id → location
const searched = ref(false)

async function doSearch() {
  if (!q.value.trim()) return
  try {
    const res = await api.searchItems(q.value.trim())
    results.value = res.items
    const m = {}
    for (const l of res.locations) m[l.id] = l
    locMap.value = m
    searched.value = true
  } catch (e) { toast(e.message, 'err') }
}

// 面包屑：基于当前房间 locations 数据拼 parent 链。
function crumbOf(locId) {
  const chain = []
  let cur = locMap.value[locId]
  while (cur) {
    chain.unshift(cur)
    cur = cur.parent_id ? locMap.value[cur.parent_id] : null
    // 搜索结果只带回直属容器，链再往上就断；完整链在详情页展示。
  }
  return chain
}

// 3D 定位：切房间（若跨房）→ 切 3D 视图 → 选中并闪烁提示。
async function locate(locId) {
  const loc = locMap.value[locId]
  if (loc?.room_id && loc.room_id !== store.currentRoomId) {
    store.currentRoomId = loc.room_id
    await reloadLocations()
  }
  store.view = 'scene'
  store.selectedId = locId
  store.flashTarget = null
  await nextTick()
  store.flashTarget = locId
}

// 大图预览。
const preview = ref(null)
function viewImage(it) {
  preview.value = it.image_url
}
</script>

<template>
  <div class="search panel">
    <div class="sbox row">
      <input v-model="q" placeholder="搜物品：如 充电器…" @keyup.enter="doSearch" autofocus />
      <button class="primary" @click="doSearch">搜索</button>
    </div>

    <div v-if="searched && !results.length" class="empty">没有找到「{{ q }}」相关物品</div>

    <div v-for="it in results" :key="it.id" class="hit" @click="select(it.location_id)">
      <div class="row">
        <img v-if="it.image_url" :src="it.image_url" class="thumb" @click.stop="viewImage(it)" />
        <strong>{{ it.name }}</strong>
        <span class="dim">×{{ it.qty }}</span>
        <span style="flex:1"></span>
        <button class="locbtn" @click.stop="locate(it.location_id)" title="3D 定位闪烁提示">📍3D定位</button>
        <span class="type-dot" :style="{ background: typeColor(locMap[it.location_id]?.location_type) }"></span>
        <span class="dim">{{ locMap[it.location_id]?.name || '?' }}</span>
      </div>
      <div class="crumb">
        <template v-for="(c, i) in crumbOf(it.location_id)" :key="c.id">
          <span v-if="i"> › </span>{{ c.name }}
        </template>
      </div>
      <div v-if="it.remark" class="dim remark">{{ it.remark }}</div>
    </div>

    <!-- 大图预览 -->
    <div v-if="preview" class="imgmask" @click="preview = null">
      <img :src="preview" class="fullimg" />
    </div>
  </div>
</template>

<style scoped>
.search { flex: 1; overflow: auto; padding: 12px; display: flex; flex-direction: column; gap: 10px; }
.sbox input { font-size: 15px; padding: 10px 12px; }
.hit {
  background: var(--bg3);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 10px 12px;
  cursor: pointer;
}
.hit:active { border-color: var(--accent); }
.locbtn {
  padding: 4px 10px;
  font-size: 12px;
  border-radius: var(--radius);
  border: 1px solid var(--accent);
  color: var(--accent);
  background: transparent;
  cursor: pointer;
  white-space: nowrap;
}
.locbtn:active { background: var(--accent); color: #fff; }
.crumb { font-size: 12px; color: var(--accent); margin-top: 5px; }
.dim { color: var(--fg-dim); font-size: 12px; }
.remark { margin-top: 3px; }
.empty { text-align: center; color: var(--fg-dim); padding: 30px 0; }
.thumb {
  width: 44px; height: 44px;
  object-fit: cover;
  border-radius: 6px;
  border: 1px solid var(--border);
  cursor: zoom-in;
}
.imgmask {
  position: fixed; inset: 0; background: rgba(20, 25, 35, .82);
  display: flex; align-items: center; justify-content: center;
  z-index: 1000; cursor: zoom-out;
}
.fullimg {
  max-width: 92vw; max-height: 92vh;
  border-radius: 8px;
  box-shadow: 0 8px 40px rgba(0,0,0,.5);
}
</style>
