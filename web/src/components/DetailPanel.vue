<script setup>
// 详情面板：面包屑、直属物品、直属子容器、几何快捷编辑。
import { ref, computed, watch } from 'vue'
import { store, selectedLoc, chainOf, depthOf, toast, reloadLocations, select } from '../store'
import { TYPE_META, typeLabel, typeColor } from '../types'
import { api } from '../api'
import LocationForm from './LocationForm.vue'
import MoveItemDialog from './MoveItemDialog.vue'

const loc = computed(() => selectedLoc())
const chain = computed(() => loc.value ? chainOf(loc.value) : [])
const items = ref([])
const children = ref([])
const editing = ref(false)
const newItem = ref({ name: '', qty: 1, remark: '', stock_group: '', min_qty: 0,
  item_w: 0, item_d: 0, item_h: 0, category: '', tags: '', is_private: 0 })
const editItem = ref(null) // { id, name, qty, remark, ... }

// 物品分类枚举（产品说明书 §4.1 物品基础信息扩展）。
const CATEGORIES = ['衣物', '书籍', '电子', '厨具', '工具', '饰品', '药品', '食材', '其他']

// tags 存储为 JSON 数组文本；编辑框用逗号分隔展示。
function parseTags(s) {
  if (!s) return ''
  try { const a = JSON.parse(s); return Array.isArray(a) ? a.join(',') : s } catch { return s }
}
function toTags(s) {
  if (!s) return ''
  const a = String(s).split(',').map(x => x.trim()).filter(Boolean)
  return a.length ? JSON.stringify(a) : ''
}
// 从后端 item 转为可编辑对象（tags 还原为逗号分隔）。
function editable(it) {
  return { ...it, tags: parseTags(it.tags) }
}
// 标签展示：JSON 数组文本 → 逗号分隔。
function tagText(s) { return parseTags(s) }

watch(() => store.selectedId, load, { immediate: true })
watch(() => store.locations.length, load)

async function load() {
  if (!loc.value) return
  try {
    ;[items.value, children.value] = await Promise.all([
      api.locationItems(loc.value.id),
      api.children(loc.value.id),
    ])
  } catch (e) {
    if (String(e.message).includes('不存在')) return
    toast(e.message, 'err')
  }
}

const canHoldItem = computed(() => loc.value && TYPE_META[loc.value.location_type]?.allowItem)
const canHaveChild = computed(() =>
  loc.value && TYPE_META[loc.value.location_type]?.allowChild && depthOf(loc.value) < 3
)

// ---- 柜内隔层网格：几层 × 几列，自动均分柜内空间 ----
const grid = ref({ rows: 3, cols: 2, generating: false })
const canGrid = computed(() =>
  loc.value && ['cabinet', 'box', 'cardbox', 'luggage'].includes(loc.value.location_type) &&
  depthOf(loc.value) < 3 && loc.value.w > 0 && loc.value.h > 0 && loc.value.d > 0
)

// 柜体自身留壳：内腔在四壁/顶底各收 2cm。
const SHELL = 0.02
async function genGrid() {
  const { rows, cols } = grid.value
  if (!(rows >= 1 && rows <= 8 && cols >= 1 && cols <= 6)) {
    toast('层数 1-8、列数 1-6', 'err')
    return
  }
  grid.value.generating = true
  try {
    const L = loc.value
    const innerW = L.w - SHELL * 2, innerH = L.h - SHELL * 2, innerD = L.d - SHELL * 2
    const cw = innerW / cols, ch = innerH / rows
    const cn = ['一', '二', '三', '四', '五', '六', '七', '八']
    for (let r = 0; r < rows; r++) {
      for (let c = 0; c < cols; c++) {
        await api.createLocation({
          room_id: L.room_id, parent_id: L.id,
          name: `${L.name}·${cn[r] || r + 1}层${c + 1}列`,
          location_type: 'drawer',
          x: +(L.x - L.w / 2 + SHELL + cw * (c + 0.5)).toFixed(3),
          y: +(L.y - L.h / 2 + SHELL + ch * (r + 0.5)).toFixed(3),
          z: +L.z.toFixed(3),
          w: +(cw - SHELL).toFixed(3), h: +(ch - SHELL).toFixed(3), d: +innerD.toFixed(3),
          rot_y: L.rot_y || 0,
        })
      }
    }
    await reloadLocations()
    toast(`已生成 ${rows}×${cols} 隔层`, 'ok')
  } catch (e) {
    toast(e.message, 'err')
  } finally {
    grid.value.generating = false
  }
}

async function addItem() {
  if (!newItem.value.name.trim()) return
  try {
    await api.createItem({
      location_id: loc.value.id,
      name: newItem.value.name.trim(),
      qty: newItem.value.qty || 1,
      remark: newItem.value.remark || '',
      stock_group: newItem.value.stock_group || '',
      min_qty: newItem.value.min_qty || 0,
      item_w: newItem.value.item_w || 0,
      item_d: newItem.value.item_d || 0,
      item_h: newItem.value.item_h || 0,
      category: newItem.value.category || '',
      tags: toTags(newItem.value.tags),
      is_private: newItem.value.is_private ? 1 : 0,
    })
    newItem.value = { name: '', qty: 1, remark: '', stock_group: '', min_qty: 0,
      item_w: 0, item_d: 0, item_h: 0, category: '', tags: '', is_private: 0 }
    await load()
    toast('物品已添加', 'ok')
  } catch (e) { toast(e.message, 'err') }
}

async function saveItem() {
  try {
    await api.updateItem(editItem.value.id, {
      location_id: loc.value.id,
      name: editItem.value.name,
      qty: editItem.value.qty || 1,
      remark: editItem.value.remark || '',
      stock_group: editItem.value.stock_group || '',
      min_qty: editItem.value.min_qty || 0,
      item_w: editItem.value.item_w || 0,
      item_d: editItem.value.item_d || 0,
      item_h: editItem.value.item_h || 0,
      category: editItem.value.category || '',
      tags: toTags(editItem.value.tags),
      is_private: editItem.value.is_private ? 1 : 0,
    })
    editItem.value = null
    await load()
    toast('物品已更新', 'ok')
  } catch (e) { toast(e.message, 'err') }
}

// ---- 库存组：用完 / 补货 ----
const consuming = ref(false)

// 用完：优先从同组补充装取 1 件；无备用则标记需补货（进待购清单）。
async function consumeItem(it) {
  if (consuming.value) return
  consuming.value = true
  try {
    const res = await api.consumeItem(it.id)
    const cands = res.candidates || []
    let sourceId = 0, sourceName = ''
    if (cands.length === 1) {
      sourceId = cands[0].id; sourceName = cands[0].name
    } else if (cands.length > 1) {
      const msg = cands.map((c, i) => `${i + 1}. ${c.name} ×${c.qty}（${c.where}）`).join('\n')
      const pick = prompt(`发现 ${cands.length} 处备用库存，选哪处取？\n${msg}`, '1')
      const n = parseInt(pick)
      if (!n || n < 1 || n > cands.length) return
      sourceId = cands[n - 1].id; sourceName = cands[n - 1].name
    } else {
      // 无备用：标记用完，进待购清单。
      if (confirm(`「${it.name}」无备用库存，标记为需补货（加入待购清单）？`)) {
        await api.markEmpty(it.id)
        await load()
        toast('已加入待购清单', 'ok')
      }
      return
    }
    await api.consumeItem(it.id, sourceId)
    await load()
    toast(`已从「${sourceName}」取 1 件`, 'ok')
  } catch (e) { toast(e.message, 'err') } finally {
    consuming.value = false
  }
}

// 补货：数量 +n。
async function restockItem(it) {
  const n = parseInt(prompt(`「${it.name}」补货数量（当前 ×${it.qty}）：`, '1'))
  if (!n || n < 1) return
  try {
    await api.restockItem(it.id, n)
    await load()
    toast(`已补货 +${n}`, 'ok')
  } catch (e) { toast(e.message, 'err') }
}

async function removeItem(it) {
  if (!confirm(`删除物品「${it.name}」？删除后可在回收站恢复。`)) return
  try {
    await api.deleteItem(it.id)
    await load()
  } catch (e) { toast(e.message, 'err') }
}

// ---- 移动物品到其他容器 ----
const movingItem = ref(null)
function openMove(it) { movingItem.value = it }
async function onMoved() {
  await load()       // 当前容器物品已减少
  await reloadLocations() // 树（若当前容器有子容器数变化）刷新
}

// ---- 图片上传/删除 ----
const imgInput = ref(null)
let imgTargetId = null

function pickImage(itemId) {
  imgTargetId = itemId
  imgInput.value?.click()
}

async function onImageFile(e) {
  const file = e.target.files?.[0]
  e.target.value = ''
  if (!file || !imgTargetId) return
  if (file.size > 10 * 1024 * 1024) { toast('图片超过 10MB 上限', 'err'); return }
  try {
    const res = await api.uploadItemImage(imgTargetId, file)
    toast('图片已上传', 'ok')
    await load()
    // 刷新后新 URL 生效（浏览器缓存同路径：URL 含时间戳不会冲突）。
    const it = items.value.find(i => i.id === imgTargetId)
    if (it) it.image_url = res.image_url
  } catch (err) { toast(err.message, 'err') }
}

async function removeImage(it) {
  if (!confirm('删除该物品图片？')) return
  try {
    await api.deleteItemImage(it.id)
    it.image_url = ''
    toast('图片已删除', 'ok')
  } catch (e) { toast(e.message, 'err') }
}

// 大图预览：遮罩层点击关闭。
const preview = ref(null)
function viewImage(it) {
  preview.value = it.image_url
}

async function removeLoc() {
  if (!loc.value) return
  if (!confirm(`删除容器「${loc.value.name}」？其子容器将一并进回收站，删除后可恢复。`)) return
  try {
    await api.deleteLocation(loc.value.id)
    store.selectedId = null
    await reloadLocations()
    toast('已删除', 'ok')
  } catch (e) { toast(e.message, 'err') }
}

function goBreadcrumb(id) {
  if (id === loc.value.id) return
  select(id)
}
</script>

<template>
  <div v-if="loc" class="detail panel">
    <div class="head">
      <div class="row">
        <span class="type-dot" :style="{ background: typeColor(loc.location_type) }"></span>
        <strong style="font-size:15px">{{ loc.name }}</strong>
        <span class="badge t">{{ typeLabel(loc.location_type) }}</span>
        <span class="badge d">深度 D{{ depthOf(loc) }}</span>
        <span style="flex:1"></span>
        <button class="small" @click="editing = true">编辑</button>
        <button class="small danger" @click="removeLoc">删除</button>
        <button v-if="store.isMobile" class="small" @click="store.view = 'tree'">←</button>
      </div>
      <!-- 面包屑：业务归属链路（需求 §5-3） -->
      <div class="crumb">
        <template v-for="(c, i) in chain" :key="c.id">
          <span v-if="i"> › </span>
          <a @click="goBreadcrumb(c.id)" :class="{ cur: c.id === loc.id }">{{ c.name }}</a>
        </template>
      </div>
    </div>

    <div class="body col">
      <!-- 几何信息 -->
      <section>
        <h4>几何（中心点 x/y/z = {{ loc.x.toFixed(2) }} / {{ loc.y.toFixed(2) }} / {{ loc.z.toFixed(2) }}，w×h×d = {{ loc.w }}×{{ loc.h }}×{{ loc.d }}，rot = {{ loc.rot_y.toFixed(2) }}）</h4>
        <p class="dim">几何与业务归属互相独立：拖拽只改摆放位置，改 parent_id 只改归属。</p>
      </section>

      <!-- 直属物品 -->
      <section>
        <h4>物品（{{ items.length }}）<span v-if="!canHoldItem" class="badge no">该类型禁止存放物品</span></h4>
        <div v-if="canHoldItem" class="row additem">
          <input v-model="newItem.name" placeholder="物品名" @keyup.enter="addItem" />
          <input v-model.number="newItem.qty" type="number" min="1" style="width:60px" title="数量" />
          <input v-model="newItem.remark" placeholder="备注" style="width:100px" />
          <input v-model.number="newItem.item_w" type="number" min="0" step="0.01" style="width:56px" placeholder="宽m" title="物品宽度（米，可选）" />
          <input v-model.number="newItem.item_d" type="number" min="0" step="0.01" style="width:56px" placeholder="深m" title="物品深度（米，可选）" />
          <input v-model.number="newItem.item_h" type="number" min="0" step="0.01" style="width:56px" placeholder="高m" title="物品高度（米，可选）" />
          <select v-model="newItem.category" style="width:72px" title="物品分类">
            <option value="">分类</option>
            <option v-for="c in CATEGORIES" :key="c" :value="c">{{ c }}</option>
          </select>
          <input v-model="newItem.tags" placeholder="标签,逗号分隔" style="width:110px" title="多个标签用逗号分隔" />
          <label class="priv" title="私有物品仅本人可见，公共物品全家共享">
            <input type="checkbox" v-model="newItem.is_private" /> 私
          </label>
          <input v-model="newItem.stock_group" placeholder="库存组(可选)" style="width:100px" title="在用装与补充装填同一组名，启用用完→取备用→补货流程" />
          <button class="small primary" @click="addItem">＋</button>
        </div>
        <table v-if="items.length" class="tbl">
          <tbody>
            <tr v-for="it in items" :key="it.id">
              <template v-if="editItem?.id === it.id">
                <td colspan="4" class="row">
                  <input v-model="editItem.name" />
                  <input v-model.number="editItem.qty" type="number" min="1" style="width:52px" />
                  <input v-model="editItem.remark" placeholder="备注" style="width:84px" />
                  <input v-model.number="editItem.item_w" type="number" min="0" step="0.01" style="width:50px" placeholder="宽m" title="宽度（米）" />
                  <input v-model.number="editItem.item_d" type="number" min="0" step="0.01" style="width:50px" placeholder="深m" title="深度（米）" />
                  <input v-model.number="editItem.item_h" type="number" min="0" step="0.01" style="width:50px" placeholder="高m" title="高度（米）" />
                  <select v-model="editItem.category" style="width:64px" title="分类">
                    <option value="">分类</option>
                    <option v-for="c in CATEGORIES" :key="c" :value="c">{{ c }}</option>
                  </select>
                  <input v-model="editItem.tags" placeholder="标签,逗号分隔" style="width:96px" title="多个标签用逗号分隔" />
                  <label class="priv" title="私有仅本人可见">
                    <input type="checkbox" v-model="editItem.is_private" /> 私
                  </label>
                  <input v-model="editItem.stock_group" placeholder="库存组(可选)" style="width:90px" title="在用装与补充装填同一组名" />
                  <input v-model.number="editItem.min_qty" type="number" min="0" style="width:52px" title="组内总量低于等于该值时进待购清单，0=不提醒" placeholder="补货线" />
                  <button class="small primary" @click="saveItem">存</button>
                  <button class="small" @click="editItem = null">取消</button>
                </td>
              </template>
              <template v-else>
                <td class="imgcell">
                  <img v-if="it.image_url" :src="it.image_url" class="thumb" @click="viewImage(it)" title="点击查看大图" />
                  <button v-else class="small imgbtn" @click="pickImage(it.id)" title="上传图片">📷</button>
                </td>
                <td>
                  {{ it.name }}
                  <span v-if="it.is_private" class="badge priv" title="私有物品（仅本人可见）">私</span>
                  <span v-if="it.category" class="badge cat" :title="'分类: ' + it.category">{{ it.category }}</span>
                  <span v-if="it.stock_group" class="badge" :class="it.qty > 1 ? 'stk' : 'stk-used'" :title="'库存组: ' + it.stock_group">{{ it.qty > 1 ? '备' : '用' }}</span>
                </td>
                <td class="dim">×{{ it.qty }}</td>
                <td class="dim ell">
                  <span v-if="it.item_w || it.item_d || it.item_h" :title="`尺寸 ${it.item_w || '-'}×${it.item_d || '-'}×${it.item_h || '-'} m`">
                    {{ it.item_w || '-' }}×{{ it.item_d || '-' }}×{{ it.item_h || '-' }}
                  </span>
                  <span v-if="it.tags" :title="'标签: ' + tagText(it.tags)">{{ tagText(it.tags) }}</span>
                  <span v-if="it.remark" class="dim">{{ it.remark }}</span>
                </td>
                <td style="width:180px; text-align:right">
                  <button v-if="it.stock_group" class="small" @click="consumeItem(it)" :disabled="consuming" title="用完：从备用处取 1 件顶上">用完</button>
                  <button v-if="it.stock_group" class="small" @click="restockItem(it)" title="补货：数量增加">补货</button>
                  <button v-if="it.image_url" class="small" @click="pickImage(it.id)" title="更换图片">换图</button>
                  <button v-if="it.image_url" class="small danger" @click="removeImage(it)" title="删除图片">🗑</button>
                  <button class="small" @click="openMove(it)" title="移动到其他容器">移动</button>
                  <button class="small" @click="editItem = editable(it)">改</button>
                  <button class="small danger" @click="removeItem(it)">删</button>
                </td>
              </template>
            </tr>
          </tbody>
        </table>
      </section>

      <!-- 直属子容器 -->
      <section>
        <h4>子容器（{{ children.length }}）<span v-if="!canHaveChild" class="badge no">{{ depthOf(loc) >= 3 ? '已达3层上限' : '该类型禁止子容器' }}</span></h4>
        <!-- 隔层网格生成：柜类容器一键均分内部空间 -->
        <div v-if="canGrid" class="gridbox row">
          <span class="dim">隔层</span>
          <input v-model.number="grid.rows" type="number" min="1" max="8" style="width:52px" title="层数（上下分）" />
          <span class="dim">层</span>
          <input v-model.number="grid.cols" type="number" min="1" max="6" style="width:52px" title="列数（左右分）" />
          <span class="dim">列</span>
          <button class="small primary" :disabled="grid.generating" @click="genGrid">
            {{ grid.generating ? '生成中…' : '⚙ 生成隔层网格' }}
          </button>
          <span class="dim" title="在柜内按层×列均分生成抽屉隔间">ⓘ</span>
        </div>
        <div v-if="children.length" class="col" style="gap:4px">
          <div v-for="c in children" :key="c.id" class="node row" @click="select(c.id)">
            <span class="type-dot" :style="{ background: typeColor(c.location_type) }"></span>
            <span>{{ c.name }}</span>
            <span class="badge t">{{ typeLabel(c.location_type) }}</span>
            <span class="badge d">D{{ depthOf(c) }}</span>
          </div>
        </div>
      </section>
    </div>

    <LocationForm
      v-if="editing"
      :edit="loc"
      :room-id="store.currentRoomId"
      @close="editing = false"
    />

    <MoveItemDialog
      v-if="movingItem"
      :item="movingItem"
      @moved="onMoved"
      @close="movingItem = null"
    />

    <input ref="imgInput" type="file" accept="image/jpeg,image/png,image/webp,image/gif" hidden @change="onImageFile" />

    <!-- 大图预览 -->
    <Teleport to="body">
      <div v-if="preview" class="imgmask" @click="preview = null">
        <img :src="preview" class="fullimg" />
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.detail { display: flex; flex-direction: column; padding: 12px; height: 100%; overflow: auto; }
.head { border-bottom: 1px solid var(--border); padding-bottom: 10px; margin-bottom: 10px; }
.crumb { margin-top: 8px; font-size: 12px; color: var(--fg-dim); }
.crumb a { color: var(--accent); cursor: pointer; }
.crumb a.cur { color: var(--fg); font-weight: 600; cursor: default; }
.body section { margin-bottom: 14px; }
h4 { font-size: 13px; margin-bottom: 6px; color: var(--fg); }
.dim { color: var(--fg-dim); font-size: 12px; }
.tbl { width: 100%; border-collapse: collapse; font-size: 13px; }
.tbl td { padding: 4px 4px; border-bottom: 1px solid var(--border); }
.ell { max-width: 110px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.additem { margin-bottom: 6px; }
.gridbox { gap: 6px; margin-bottom: 8px; flex-wrap: wrap; }
.gridbox input { width: 52px; padding: 4px 6px; }
.badge.t { background: var(--bg3); color: var(--fg-dim); }
.badge.d { background: #fdf3d5; color: #8a6417; }
.badge.no { background: #fdeaea; color: var(--danger); font-size: 11px; }
.node { cursor: pointer; padding: 4px 6px; border-radius: 6px; }
.node:hover { background: var(--bg3); }
.imgcell { width: 44px; padding: 2px !important; }
.thumb {
  width: 40px; height: 40px;
  object-fit: cover;
  border-radius: 6px;
  border: 1px solid var(--border);
  cursor: zoom-in;
  display: block;
}
.imgbtn { font-size: 15px; padding: 4px 8px; }
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
.badge.stk { background: #fff4e0; color: #b25e09; }
.badge.stk-used { background: #e8f3ff; color: #2563c9; }
.badge.priv { background: #fdeaea; color: #d1465c; }
.badge.cat { background: #eaf6ee; color: #1f7a4d; }
.priv { display: inline-flex; align-items: center; gap: 2px; font-size: 12px; color: var(--fg-dim); white-space: nowrap; }
</style>
