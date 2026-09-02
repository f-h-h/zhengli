<script setup>
// 待购清单：库存组总量 ≤ 阈值的物品汇总。
import { ref } from 'vue'
import { toast } from '../store'
import { api } from '../api'

const open = ref(false)
const entries = ref([])
const loading = ref(false)

async function show() {
  open.value = true
  loading.value = true
  try {
    entries.value = await api.shoppingList()
  } catch (e) { toast(e.message, 'err') }
  loading.value = false
}

defineExpose({ show })
</script>

<template>
  <button class="small" @click="show">🛒 待购</button>

  <teleport to="body">
    <div v-if="open" class="mask" @click.self="open = false">
      <div class="dialog panel">
        <div class="row" style="align-items:baseline">
          <h3>待购清单（{{ entries.length }}）</h3>
          <span class="dim" style="flex:1">库存组总量 ≤ 补货线时自动出现</span>
          <button class="small" @click="open = false">关闭</button>
        </div>

        <div v-if="loading" class="dim" style="padding:20px;text-align:center">加载中…</div>
        <div v-else-if="!entries.length" class="dim" style="padding:20px;text-align:center">没有需要补货的物品</div>

        <div v-else class="list">
          <div v-for="e in entries" :key="e.stock_group" class="entry row">
            <div class="grow">
              <div class="name">{{ e.name }} <span class="dim sub">（组：{{ e.stock_group }}）</span></div>
              <div class="dim sub">现存 {{ e.total }} · 补货线 {{ e.min_qty }}</div>
            </div>
            <span class="badge need">建议买 {{ e.suggest }}</span>
          </div>
        </div>
        <p class="dim tip">买回来后到对应位置点「补货」录入；提示：在用装与补充装需填同一「库存组」名称。</p>
      </div>
    </div>
  </teleport>
</template>

<style scoped>
.mask {
  position: fixed; inset: 0; background: rgba(40, 50, 70, .38);
  display: flex; align-items: center; justify-content: center; z-index: 100;
}
.dialog { padding: 16px; width: min(520px, 94vw); max-height: 82vh; display: flex; flex-direction: column; }
.dialog h3 { margin-bottom: 10px; }
.list { overflow: auto; }
.entry { padding: 8px 0; gap: 8px; align-items: center; border-bottom: 1px solid var(--border); }
.entry:last-child { border-bottom: none; }
.name { font-weight: 500; }
.sub { font-size: 12px; }
.badge.need { background: #ffece8; color: #c0392b; white-space: nowrap; }
.tip { font-size: 12px; margin-top: 10px; }
</style>
