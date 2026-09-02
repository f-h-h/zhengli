<script setup>
// CSV 导入导出：三套模板导入 + 四种导出。
import { ref } from 'vue'
import { store, reloadLocations, toast } from '../store'
import { api, downloadExport } from '../api'

const fileInput = ref(null)
const importKind = ref('locations')
const importMode = ref('add')
const busy = ref(false)

function pickFile(kind) {
  importKind.value = kind
  fileInput.value?.click()
}

async function onFile(e) {
  const file = e.target.files?.[0]
  e.target.value = ''
  if (!file) return
  busy.value = true
  try {
    const text = await file.text()
    let res
    if (importKind.value === 'locations') {
      res = await api.importLocations(store.currentRoomId, text, importMode.value)
    } else {
      res = await api.importItems(text)
    }
    const errMsgs = (res.errors || []).map(er => `第${er.row}行：${er.msg}`)
    if (errMsgs.length) {
      toast(`新增${res.created} 更新${res.updated}；${errMsgs.length}条失败`, 'warn')
      toast(errMsgs.slice(0, 4).join('；') + (errMsgs.length > 4 ? ' 等' : ''), 'warn')
    } else {
      toast(`导入成功：新增${res.created} 更新${res.updated}`, 'ok')
    }
    await reloadLocations()
  } catch (err) {
    toast(err.message, 'err')
  } finally {
    busy.value = false
  }
}

function dl(kind) {
  if (!store.currentRoomId) return
  downloadExport(store.currentRoomId, kind)
}
</script>

<template>
  <details class="ie">
    <summary>导入/导出</summary>
    <div class="col menu">
      <div class="row">
        <select v-model="importMode" style="width:auto" title="导入模式">
          <option value="add">增量新增/按ID更新</option>
          <option value="replace">清空房间重导入</option>
        </select>
        <button class="small" :disabled="busy" @click="pickFile('locations')">导入 locations.csv</button>
        <button class="small" :disabled="busy" @click="pickFile('items')">导入 items.csv</button>
      </div>
      <div class="row">
        <button class="small" @click="dl('locations')">导出 locations.csv</button>
        <button class="small" @click="dl('items')">导出物品清单</button>
        <button class="small" @click="dl('snapshot')">导出布局快照</button>
      </div>
    </div>
    <input ref="fileInput" type="file" accept=".csv,text/csv" hidden @change="onFile" />
  </details>
</template>

<style scoped>
.ie { font-size: 13px; }
.ie summary { cursor: pointer; color: var(--fg-dim); padding: 3px 6px; user-select: none; }
.menu {
  position: absolute;
  top: calc(100% + 6px);
  right: 8px;
  background: var(--bg3);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 10px;
  z-index: 50;
  box-shadow: 0 6px 22px rgba(0,0,0,.5);
}
</style>
