<script setup>
// 顶栏：房间切换、房间编辑、新建房间。
import { ref, computed } from 'vue'
import { store, currentRoom, selectRoom, reloadLocations, toast } from '../store'
import { api } from '../api'
import ImportExport from './ImportExport.vue'
import TrashDialog from './TrashDialog.vue'
import ShoppingDialog from './ShoppingDialog.vue'

const editing = ref(false)
const form = ref({ name: '', room_w: 4, room_d: 3, room_h: 2.8 })
const creating = ref(false)

const room = computed(() => currentRoom())

function startEdit() {
  if (!room.value) return
  form.value = { ...room.value }
  editing.value = true
}

async function saveRoom() {
  try {
    await api.updateRoom(room.value.id, {
      name: form.value.name, room_w: +form.value.room_w,
      room_d: +form.value.room_d, room_h: +form.value.room_h,
    })
    editing.value = false
    await refreshRooms()
    toast('房间已更新', 'ok')
  } catch (e) { toast(e.message, 'err') }
}

async function createRoom() {
  try {
    const r = await api.createRoom({
      name: form.value.name, room_w: +form.value.room_w,
      room_d: +form.value.room_d, room_h: +form.value.room_h,
    })
    creating.value = false
    await refreshRooms()
    await selectRoom(r.id)
    toast('房间已创建', 'ok')
  } catch (e) { toast(e.message, 'err') }
}

async function refreshRooms() {
  store.rooms = await api.listRooms()
}

async function onSwitch(e) {
  try {
    await selectRoom(+e.target.value)
  } catch (err) { toast(err.message, 'err') }
}
</script>

<template>
  <header class="bar panel">
    <div class="brand">
      <svg class="brand-ico" viewBox="0 0 64 64" aria-hidden="true">
        <rect width="64" height="64" rx="14" fill="#2f6fed"/>
        <rect x="20" y="14" width="24" height="7" rx="3.5" fill="#dbe7fd"/>
        <rect x="12" y="20" width="40" height="30" rx="5" fill="#ffffff"/>
        <line x1="12" y1="35" x2="52" y2="35" stroke="#2f6fed" stroke-width="2.5"/>
        <rect x="18" y="39" width="12" height="4" rx="2" fill="#2f6fed"/>
        <rect x="34" y="39" width="12" height="4" rx="2" fill="#2f6fed"/>
      </svg>
      收纳管理
    </div>

    <div class="row grow">
      <select v-if="store.rooms.length" :value="store.currentRoomId" @change="onSwitch" style="width:160px">
        <option v-for="r in store.rooms" :key="r.id" :value="r.id">{{ r.name }}</option>
      </select>
      <button v-if="room" class="small" @click="startEdit">编辑房间</button>
      <button class="small" @click="creating = true; form = { name: '', room_w: 4, room_d: 3, room_h: 2.8 }">＋新建房间</button>
      <ImportExport v-if="room" />
      <TrashDialog ref="trashDlg" />
      <ShoppingDialog ref="shopDlg" />
    </div>

    <!-- 房间编辑弹层 -->
    <div v-if="editing || creating" class="mask" @click.self="editing = creating = false">
      <div class="dialog panel">
        <h3>{{ editing ? '编辑房间' : '新建房间' }}</h3>
        <div class="col">
          <label>名称 <input v-model="form.name" /></label>
          <div class="row">
            <label class="grow">长 X (m) <input type="number" step="0.1" v-model.number="form.room_w" /></label>
            <label class="grow">宽 Z (m) <input type="number" step="0.1" v-model.number="form.room_d" /></label>
            <label class="grow">高 Y (m) <input type="number" step="0.1" v-model.number="form.room_h" /></label>
          </div>
          <div class="row" style="justify-content:flex-end">
            <button @click="editing = creating = false">取消</button>
            <button class="primary" @click="editing ? saveRoom() : createRoom()">保存</button>
          </div>
        </div>
      </div>
    </div>
  </header>
</template>

<style scoped>
.bar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 14px;
  margin: 8px 8px 0;
}
.brand { font-weight: 600; white-space: nowrap; display: flex; align-items: center; gap: 6px; }
.brand-ico { width: 22px; height: 22px; flex-shrink: 0; border-radius: 5px; }
.grow { flex: 1; }
label { font-size: 12px; color: var(--fg-dim); display: flex; flex-direction: column; gap: 4px; }
.mask {
  position: fixed; inset: 0; background: rgba(40, 50, 70, .38);
  display: flex; align-items: center; justify-content: center; z-index: 100;
}
.dialog { padding: 18px; width: min(480px, 92vw); }
.dialog h3 { margin-bottom: 14px; }
</style>
