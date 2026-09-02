<script setup>
// 新建/编辑容器表单：名称、类型、parent_id、尺寸坐标。
// 支持预设（presets.js）：预填真实尺寸 + 建议位置；置物架/床头柜为组合批量创建。
// 简易采集字段 corner_x/corner_z/bottom_y 由本表单直接换算中心点（模式B）。
import { reactive, computed } from 'vue'
import { store, reloadLocations, toast, depthOf, currentRoom } from '../store'
import { TYPE_META, TYPE_KEYS, typeLabel } from '../types'
import { api } from '../api'

const props = defineProps({
  parent: { type: Object, default: null },  // 新建时的父容器
  roomId: { type: Number, required: true },
  edit: { type: Object, default: null },    // 编辑模式传入现有 location
  preset: { type: Object, default: null },  // 模型库预设（仅新建）
})
const emit = defineEmits(['close'])

const isEdit = computed(() => !!props.edit)
const isCombo = computed(() => !isEdit.value && !!props.preset?.special)
// 门上只能贴挂钩：父容器是门时锁定类型。
const parentIsDoor = computed(() => props.parent?.location_type === 'door')

const f = reactive(initForm())

// initForm 需要 parentIsDoor 已就绪（computed 定义于其前）。
if (parentIsDoor.value && !isEdit.value) f.location_type = 'wall_mount'

// 建议位置：普通预设按已有数量错开摆放；墙面预设贴前墙（z=0），高度取预设值。
function suggestPos(p) {
  const room = currentRoom()
  const y = p.y ?? p.h / 2
  if (!room) return { x: +(p.w / 2 + 0.1).toFixed(2), y, z: +(p.d / 2 + 0.1).toFixed(2) }
  if (p.wall) return { x: +(room.room_w / 2).toFixed(2), y, z: +(p.d / 2).toFixed(2) }
  const n = store.locations.length
  const xSpan = Math.max(room.room_w - p.w - 0.2, 0)
  const zSpan = Math.max(room.room_d - p.d - 0.2, 0)
  return {
    x: +(p.w / 2 + 0.1 + (n % 4) * (xSpan / 4)).toFixed(2),
    y,
    z: +(p.d / 2 + 0.1 + Math.floor(n / 4) * (zSpan / 3)).toFixed(2),
  }
}

function initForm() {
  if (props.edit) {
    const e = props.edit
    return {
      name: e.name, location_type: e.location_type,
      parent_id: e.parent_id || 0,
      // 编辑模式直接用中心点。
      x: e.x, y: e.y, z: e.z, w: e.w, h: e.h, d: e.d, rot_y: e.rot_y,
      color: e.color, is_cardbox: e.is_cardbox,
      cornerMode: false, corner_x: 0, corner_z: 0, bottom_y: 0, rot_y_deg: 0,
      layers: 4,
    }
  }
  if (props.preset) {
    const p = props.preset
    const pos = suggestPos(p)
    return {
      name: p.name, location_type: p.type,
      parent_id: props.parent ? props.parent.id : 0,
      x: pos.x, y: pos.y, z: pos.z,
      w: p.w, h: p.h, d: p.d,
      rot_y: 0, color: p.color || '', is_cardbox: p.type === 'cardbox' ? 1 : 0,
      cornerMode: false, corner_x: 0, corner_z: 0, bottom_y: 0, rot_y_deg: 0,
      layers: p.layers || 4,
    }
  }
  // 空白新建：默认尺寸取父容器的 60%，坐标落在父容器中心（用户再微调）。
  return {
    name: '', location_type: 'box',
    parent_id: props.parent ? props.parent.id : 0,
    x: props.parent ? +props.parent.x.toFixed(3) : 0,
    y: props.parent ? +props.parent.y.toFixed(3) : 0,
    z: props.parent ? +props.parent.z.toFixed(3) : 0,
    w: props.parent?.w > 0 ? +(props.parent.w * 0.6).toFixed(3) : 0.4,
    h: props.parent?.h > 0 ? +(props.parent.h * 0.6).toFixed(3) : 0.3,
    d: props.parent?.d > 0 ? +(props.parent.d * 0.6).toFixed(3) : 0.4,
    rot_y: 0, color: '', is_cardbox: 0,
    cornerMode: false, corner_x: 0, corner_z: 0, bottom_y: 0, rot_y_deg: 0,
    layers: 4,
  }
}

// 可选父容器：同房间、允许子容器、且深度 < 3（规格 §3.2-3）。
// 例外：创建挂钩(wall_mount)时门也可作为父容器。
const parentOptions = computed(() =>
  store.locations.filter(l =>
    (TYPE_META[l.location_type]?.allowChild || (l.location_type === 'door' && f.location_type === 'wall_mount')) &&
    depthOf(l) < 3 && l.id !== props.edit?.id
  )
)

// 组合预设的批量记录：置物架 = 架体 + N 层板；床头柜 = 台面 + 抽屉（规格 §4）。
// 成员共享 geom_group：拖拽时任一成员，组内整体平移（Scene3D 联动）。
function comboRecords() {
  const base = { room_id: props.roomId, parent_id: null, rot_y: +f.rot_y, geom_group: crypto.randomUUID() }
  if (props.preset.special === 'shelf') {
    const n = Math.max(2, Math.min(6, +f.layers || 4))
    const spacing = (+f.h - 0.1) / n
    const recs = [{
      ...base, name: `${f.name}·架体`, location_type: 'furniture',
      x: +f.x, y: +f.h / 2, z: +f.z, w: +f.w, h: +f.h, d: +f.d,
      color: f.color || undefined,
    }]
    for (let i = 1; i <= n; i++) {
      recs.push({
        ...base, name: `${f.name}·L${i}`, location_type: 'shelf_layer',
        x: +f.x, y: +(0.05 + (i - 1) * spacing + 0.0175).toFixed(3), z: +f.z,
        w: +f.w, h: 0.035, d: Math.max(+f.d - 0.04, 0.05),
      })
    }
    return recs
  }
  if (props.preset.special === 'nightstand') {
    const topH = 0.04
    return [
      { ...base, name: `${f.name}·台面`, location_type: 'table',
        x: +f.x, y: +f.h - topH / 2, z: +f.z, w: +f.w, h: topH, d: +f.d },
      { ...base, name: `${f.name}·抽屉`, location_type: 'drawer',
        x: +f.x, y: +((+f.h - topH) / 2), z: +f.z, w: +f.w, h: +f.h - topH, d: +f.d },
    ]
  }
  return null
}

function buildBody() {
  const body = {
    room_id: props.roomId,
    name: f.name.trim(),
    location_type: f.location_type,
    parent_id: +f.parent_id || null,
    color: f.color || undefined,
    is_cardbox: f.location_type === 'cardbox' ? 1 : f.is_cardbox,
  }
  if (f.cornerMode && !isEdit.value) {
    // 简易采集换算（规格 §6 模式B）。
    body.w = +f.w; body.h = +f.h; body.d = +f.d
    body.x = +f.corner_x + body.w / 2
    body.z = +f.corner_z + body.d / 2
    body.y = +f.bottom_y + body.h / 2
    body.rot_y = (+f.rot_y_deg || 0) * Math.PI / 180
  } else {
    body.x = +f.x; body.y = +f.y; body.z = +f.z
    body.w = +f.w; body.h = +f.h; body.d = +f.d
    body.rot_y = +f.rot_y
  }
  return body
}

async function save() {
  if (!f.name.trim()) { toast('名称不能为空', 'err'); return }
  try {
    if (isEdit.value) {
      await api.updateLocation(props.edit.id, buildBody())
      toast('已保存', 'ok')
    } else if (isCombo.value) {
      // 组合预设：逐条创建，任一失败即停止并报错。
      const recs = comboRecords()
      for (const r of recs) await api.createLocation(r)
      toast(`已创建 ${recs.length} 条`, 'ok')
    } else {
      await api.createLocation(buildBody())
      toast('已创建', 'ok')
    }
    await reloadLocations()
    emit('close')
  } catch (e) {
    toast(e.message, 'err')
  }
}
</script>

<template>
  <div class="mask" @click.self="emit('close')">
    <div class="dialog panel">
      <h3>{{
        isEdit ? `编辑：${edit.name}`
        : isCombo ? `新增：${preset.name}（组合）`
        : props.preset ? `新增：${preset.name}`
        : props.parent ? `在「${parent.name}」内新建` : '新建顶层容器'
      }}</h3>
      <div class="col">
        <div class="row">
          <label class="grow">名称 <input v-model="f.name" /></label>
          <label style="width:140px">类型
            <select v-model="f.location_type" :disabled="parentIsDoor && !isEdit" :title="parentIsDoor ? '门上只能贴挂钩' : ''">
              <option v-for="k in TYPE_KEYS" :key="k" :value="k">{{ typeLabel(k) }}</option>
            </select>
          </label>
        </div>

        <label v-if="!isCombo">业务归属 parent_id
          <select v-model="f.parent_id" number>
            <option :value="0">（顶层 / 无归属）</option>
            <option v-for="p in parentOptions" :key="p.id" :value="p.id">
              {{ p.name }}（{{ typeLabel(p.location_type) }} D{{ depthOf(p) }}）
            </option>
          </select>
        </label>
        <div v-else class="dim combo-hint">
          组合预设自动创建多条顶层记录，各记录的归属可在创建后单独调整。
        </div>

        <!-- 置物架层数 -->
        <div v-if="preset?.special === 'shelf' && !isEdit" class="row">
          <label class="grow">层数
            <input type="number" min="2" max="6" v-model.number="f.layers" />
          </label>
          <span class="dim hint">将创建架体 + {{ Math.max(2, Math.min(6, +f.layers || 4)) }} 块层板</span>
        </div>
        <div v-if="preset?.special === 'nightstand' && !isEdit" class="dim hint">
          将创建「台面 table」+「抽屉 drawer」两条记录（规格 §4 床头柜映射）
        </div>

        <template v-if="!isEdit">
          <label class="row" style="flex-direction:row">
            <input type="checkbox" v-model="f.cornerMode" />
            简易采集模式（墙角偏移 + 底部离地，自动换算中心点）
          </label>

          <div v-if="f.cornerMode" class="row">
            <label class="grow">corner_x <input type="number" step="0.01" v-model.number="f.corner_x" /></label>
            <label class="grow">corner_z <input type="number" step="0.01" v-model.number="f.corner_z" /></label>
            <label class="grow">bottom_y <input type="number" step="0.01" v-model.number="f.bottom_y" /></label>
            <label class="grow">rot_y_deg <input type="number" step="1" v-model.number="f.rot_y_deg" /></label>
          </div>
        </template>

        <div v-if="!f.cornerMode || isEdit" class="row">
          <label class="grow">x 中心 <input type="number" step="0.01" v-model.number="f.x" /></label>
          <label class="grow">y 中心 <input type="number" step="0.01" v-model.number="f.y" /></label>
          <label class="grow">z 中心 <input type="number" step="0.01" v-model.number="f.z" /></label>
          <label class="grow">rot_y(rad) <input type="number" step="0.01" v-model.number="f.rot_y" /></label>
        </div>
        <div class="row">
          <label class="grow">w 宽X <input type="number" step="0.01" v-model.number="f.w" /></label>
          <label class="grow">h 高Y <input type="number" step="0.01" v-model.number="f.h" /></label>
          <label class="grow">d 深Z <input type="number" step="0.01" v-model.number="f.d" /></label>
        </div>

        <div class="row" style="justify-content:flex-end">
          <button @click="emit('close')">取消</button>
          <button class="primary" @click="save">保存</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.mask {
  position: fixed; inset: 0; background: rgba(40, 50, 70, .38);
  display: flex; align-items: center; justify-content: center; z-index: 100;
}
.dialog { padding: 18px; width: min(560px, 94vw); max-height: 88vh; overflow: auto; }
.dialog h3 { margin-bottom: 14px; }
label { font-size: 12px; color: var(--fg-dim); display: flex; flex-direction: column; gap: 4px; }
label.row { flex-direction: row; align-items: center; gap: 6px; color: var(--fg); font-size: 13px; }
.dim { color: var(--fg-dim); font-size: 12px; }
.combo-hint { margin: -2px 0; }
.hint { font-size: 12px; }
</style>
