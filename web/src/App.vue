<script setup>
import { onMounted, onUnmounted, computed, defineAsyncComponent } from 'vue'
import { store, loadRooms, loadHousehold, selectRoom } from './store'
import RoomBar from './components/RoomBar.vue'
import TreePanel from './components/TreePanel.vue'
// three.js 体积大：动态加载拆出主包，首屏只渲染 UI。
const Scene3D = defineAsyncComponent(() => import('./components/Scene3D.vue'))
import DetailPanel from './components/DetailPanel.vue'
import SearchPanel from './components/SearchPanel.vue'

const resize = () => { store.isMobile = window.innerWidth < 768 }
onMounted(async () => {
  window.addEventListener('resize', resize)
  try {
    await Promise.all([loadRooms(true), loadHousehold()])
  } catch (e) {
    console.error(e)
  }
})
onUnmounted(() => window.removeEventListener('resize', resize))

const showDetail = computed(() => store.selectedId && (store.isMobile ? store.view === 'detail' : true))

// 移动端底部标签。
const tabs = [
  { key: 'search', label: '搜索' },
  { key: 'tree', label: '收纳树' },
  { key: 'scene', label: '3D' },
]
</script>

<template>
  <div class="app" :class="{ mobile: store.isMobile }">
    <div class="housebar" v-if="store.household" :title="'当前家：' + store.household.name">
      <span class="hico">🏠</span>
      <span class="hname">{{ store.household.name }}</span>
    </div>
    <RoomBar />

    <div v-if="store.isMobile" class="mobile-body">
      <Transition name="view" mode="out-in">
        <SearchPanel v-if="store.view === 'search'" key="search" />
        <TreePanel v-else-if="store.view === 'tree'" key="tree" />
        <Scene3D v-else-if="store.view === 'scene'" key="scene" />
        <DetailPanel v-else-if="store.view === 'detail'" key="detail" />
      </Transition>
      <nav class="tabbar">
        <button
          v-for="t in tabs" :key="t.key"
          :class="{ active: store.view === t.key }"
          @click="store.view = t.key"
        >{{ t.label }}</button>
      </nav>
    </div>

    <div v-else class="main">
      <TreePanel class="left" />
      <div class="center">
        <Scene3D />
      </div>
      <DetailPanel v-if="showDetail" class="right" />
    </div>

    <div class="toast-wrap">
      <div v-for="t in store.toasts" :key="t.id" class="toast" :class="t.type">{{ t.msg }}</div>
    </div>
  </div>
</template>

<style scoped>
.app { height: 100%; display: flex; flex-direction: column; }
/* 家条：顶部展示当前数据归属单位 */
.housebar {
  display: flex; align-items: center; gap: 6px;
  padding: 4px 14px; font-size: 12px; color: var(--fg-dim);
  background: var(--bg2); border-bottom: 1px solid var(--border);
}
.hico { font-size: 13px; }
.hname { font-weight: 600; color: var(--fg); }
.main {
  flex: 1;
  display: grid;
  grid-template-columns: 300px 1fr auto;
  min-height: 0;
  gap: 8px;
  padding: 8px;
}
.left, .right {
  min-height: 0;
  overflow: auto;
}
.center {
  position: relative;
  min-width: 0;
  min-height: 0;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  overflow: hidden;
}
.right { width: 360px; }

.mobile-body { flex: 1; display: flex; flex-direction: column; min-height: 0; padding: 8px; gap: 8px; }
.mobile-body > :first-child:not(nav) { flex: 1; min-height: 0; }
.tabbar {
  display: flex;
  gap: 8px;
  padding-bottom: 2px;
}
.tabbar button { flex: 1; padding: 10px 0; }
.tabbar button.active { background: var(--accent); border-color: var(--accent); color: #fff; }

/* 移动端视图切换：轻微位移动画 */
.view-enter-active, .view-leave-active { transition: opacity .16s ease, transform .16s ease; }
.view-enter-from { opacity: 0; transform: translateX(14px); }
.view-leave-to { opacity: 0; transform: translateX(-14px); }
</style>
