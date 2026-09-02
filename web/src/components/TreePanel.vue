<script setup>
// 收纳树：按业务归属 parent_id 组织（需求 §5-3 面包屑导航同源）。
import { computed, ref } from 'vue'
import { store, depthOf, select, reloadLocations, toast } from '../store'
import { TYPE_META, typeLabel, typeColor } from '../types'
import { api } from '../api'
import LocationForm from './LocationForm.vue'
import PresetGallery from './PresetGallery.vue'

// 构建树：根节点 + parent 挂载。
const tree = computed(() => {
  const byParent = new Map()
  const roots = []
  for (const l of store.locations) {
    const pid = l.parent_id || 0
    if (!pid) { roots.push(l); continue }
    if (!byParent.has(pid)) byParent.set(pid, [])
    byParent.get(pid).push(l)
  }
  function node(l) {
    return { loc: l, children: (byParent.get(l.id) || []).map(node) }
  }
  // 未找到父（数据异常）也放根，避免丢节点。
  const seen = new Set()
  for (const [pid] of byParent) seen.add(pid)
  for (const l of store.locations) {
    if (l.parent_id && !seen.has(l.parent_id) && !store.locations.some(x => x.id === l.parent_id)) {
      roots.push(l)
    }
  }
  return roots.map(node)
})

const expanded = ref(new Set())

function toggle(id) {
  if (expanded.value.has(id)) expanded.value.delete(id)
  else expanded.value.add(id)
}

const showForm = ref(false)
const formParent = ref(null) // null = 根级
const formPreset = ref(null) // 模型库预设

const showGallery = ref(false)

function addChild(parent) {
  formParent.value = parent
  formPreset.value = null
  showForm.value = true
}

// 模型库选中预设（null = 空白自定义）→ 打开预填表单。
function pickPreset(preset) {
  showGallery.value = false
  formParent.value = null
  formPreset.value = preset || null
  showForm.value = true
}

async function removeLoc(l) {
  if (!confirm(`删除「${l.name}」？其子容器将一并进回收站，删除后可恢复。`)) return
  try {
    await api.deleteLocation(l.id)
    if (store.selectedId === l.id) store.selectedId = null
    await reloadLocations()
    toast('已移入回收站', 'ok')
  } catch (e) { toast(e.message, 'err') }
}

// 门：唯一例外是可贴挂钩（depth=3 限制照旧）。
function canAddChild(l) {
  if (depthOf(l) >= 3) return false
  return TYPE_META[l.location_type]?.allowChild || l.location_type === 'door'
}
</script>

<template>
  <div class="tree panel">
    <div class="head row">
      <strong>收纳树</strong>
      <span class="dim">{{ store.locations.length }} 个容器</span>
      <span style="flex:1"></span>
      <button class="small primary" @click="showGallery = true">＋顶层</button>
    </div>

    <div class="list">
      <template v-if="tree.length">
        <div v-for="n in tree" :key="n.loc.id">
          <div
            class="node"
            :class="{ sel: n.loc.id === store.selectedId }"
            :style="{ paddingLeft: '8px' }"
            @click="select(n.loc.id)"
          >
            <span class="twist" v-if="n.children.length" @click.stop="toggle(n.loc.id)">
              {{ expanded.has(n.loc.id) ? '▾' : '▸' }}
            </span>
            <span v-else class="twist dim2">·</span>
            <span class="type-dot" :style="{ background: typeColor(n.loc.location_type) }"></span>
            <span class="name">{{ n.loc.name }}</span>
            <span class="badge t">{{ typeLabel(n.loc.location_type) }}</span>
            <span class="badge d" v-if="depthOf(n.loc) > 0">D{{ depthOf(n.loc) }}</span>
          </div>
          <div v-if="expanded.has(n.loc.id)" class="kids">
            <div
              v-for="k in n.children" :key="k.loc.id"
              class="node"
              :class="{ sel: k.loc.id === store.selectedId }"
              style="padding-left: 26px"
              @click="select(k.loc.id)"
            >
              <span class="twist" v-if="k.children.length" @click.stop="toggle(k.loc.id)">
                {{ expanded.has(k.loc.id) ? '▾' : '▸' }}
              </span>
              <span v-else class="twist dim2">·</span>
              <span class="type-dot" :style="{ background: typeColor(k.loc.location_type) }"></span>
              <span class="name">{{ k.loc.name }}</span>
              <span class="badge t">{{ typeLabel(k.loc.location_type) }}</span>
              <span class="badge d" v-if="depthOf(k.loc) > 0">D{{ depthOf(k.loc) }}</span>
            </div>
            <!-- depth=3 隐藏新建子按钮（需求 §5-6）；门只露出贴挂钩入口 -->
            <button
              v-if="canAddChild(n.loc)"
              class="small add" style="margin-left: 26px"
              @click="addChild(n.loc)"
            >{{ n.loc.location_type === 'door' ? '＋挂钩' : '＋子容器' }}</button>
          </div>
          <button
            v-if="TYPE_META[n.loc.location_type]?.allowChild && depthOf(n.loc) < 3 && !expanded.has(n.loc.id) && n.children.length"
            class="small add" style="margin-left: 26px"
            @click="addChild(n.loc)"
          >＋子容器</button>
        </div>
      </template>
      <div v-else class="empty">
        暂无容器。<br/>点击「＋顶层」从模型库添加，或用 CSV 批量导入。
      </div>
    </div>

    <PresetGallery v-if="showGallery" @close="showGallery = false" @pick="pickPreset" />

    <LocationForm
      v-if="showForm"
      :parent="formParent"
      :preset="formPreset"
      :room-id="store.currentRoomId"
      @close="showForm = false"
    />
  </div>
</template>

<style scoped>
.tree { display: flex; flex-direction: column; padding: 10px; height: 100%; }
.head { margin-bottom: 8px; }
.dim { color: var(--fg-dim); font-size: 12px; }
.dim2 { color: var(--fg-dim); }
.list { flex: 1; overflow: auto; }
.node {
  display: flex;
  align-items: center;
  gap: 5px;
  padding: 5px 8px;
  border-radius: 6px;
  cursor: pointer;
  white-space: nowrap;
}
.node:hover { background: var(--bg3); }
.node.sel { background: var(--accent-weak); }
.twist { width: 14px; cursor: pointer; text-align: center; color: var(--fg-dim); }
.name { overflow: hidden; text-overflow: ellipsis; }
.badge.t { background: var(--bg3); color: var(--fg-dim); }
.badge.d { background: #fdf3d5; color: #8a6417; }
.kids { }
.add { margin-top: 2px; margin-bottom: 4px; opacity: .75; }
.empty { color: var(--fg-dim); padding: 20px; text-align: center; line-height: 1.8; }
</style>
