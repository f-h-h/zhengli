<script setup>
// 基础模型库选择器：点选预设后交给 LocationForm 预填并允许微调尺寸。
import { ref } from 'vue'
import { PRESET_GROUPS } from '../presets'
import { typeLabel } from '../types'

const emit = defineEmits(['close', 'pick'])

// 置物架层数：默认 4，范围 2-6。
const layers = ref(4)

function pick(preset) {
  if (preset && preset.special === 'shelf') {
    emit('pick', { ...preset, layers: layers.value })
    return
  }
  emit('pick', preset)
}
</script>

<template>
  <div class="mask" @click.self="emit('close')">
    <div class="dialog panel">
      <div class="head row">
        <h3>基础模型库</h3>
        <span class="dim">选中后可修改尺寸和位置</span>
        <span style="flex:1"></span>
        <button class="small" @click="emit('close')">✕</button>
      </div>

      <div class="groups">
        <div v-for="g in PRESET_GROUPS" :key="g.title" class="group">
          <div class="gtitle">{{ g.title }}</div>
          <div class="cards">
            <div v-for="p in g.items" :key="p.key" class="card" @click="pick(p)">
              <div class="icon">{{ p.icon }}</div>
              <div class="pname">{{ p.name }}</div>
              <div class="ptype">{{ typeLabel(p.type) }}</div>
              <div class="pdim">{{ p.w }}×{{ p.h }}×{{ p.d }} m</div>
              <!-- 置物架：层数选择（点击卡片本身不带层数，需先选层数） -->
              <div v-if="p.special === 'shelf'" class="lay" @click.stop>
                层数
                <select v-model.number="layers">
                  <option v-for="n in [2, 3, 4, 5, 6]" :key="n" :value="n">{{ n }}</option>
                </select>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="foot row">
        <span class="dim" style="flex:1">找不到？从空白自定义开始。</span>
        <button @click="emit('close')">取消</button>
        <button class="primary" @click="pick(null)">空白自定义</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.mask {
  position: fixed; inset: 0; background: rgba(40, 50, 70, .38);
  display: flex; align-items: center; justify-content: center; z-index: 100;
}
.dialog { padding: 16px; width: min(680px, 94vw); max-height: 88vh; display: flex; flex-direction: column; }
.head { margin-bottom: 10px; }
.head h3 { font-size: 16px; }
.dim { color: var(--fg-dim); font-size: 12px; }
.groups { flex: 1; overflow: auto; }
.gtitle {
  font-size: 12px; color: var(--fg-dim); margin: 10px 0 6px;
  border-bottom: 1px solid var(--border); padding-bottom: 4px;
}
.cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(120px, 1fr)); gap: 8px; }
.card {
  border: 1px solid var(--border); border-radius: var(--radius);
  padding: 10px 8px; text-align: center; cursor: pointer; background: #fff;
  transition: border-color .12s, box-shadow .12s;
}
.card:hover { border-color: var(--accent); box-shadow: 0 2px 10px rgba(47, 111, 237, .15); }
.icon { font-size: 26px; line-height: 1.2; }
.pname { font-size: 13px; font-weight: 600; margin-top: 4px; }
.ptype { font-size: 11px; color: var(--fg-dim); }
.pdim { font-size: 11px; color: var(--fg-dim); margin-top: 2px; }
.lay { margin-top: 6px; font-size: 11px; color: var(--fg); }
.lay select { width: 60px; padding: 2px 4px; font-size: 12px; }
.foot { margin-top: 12px; }
</style>
