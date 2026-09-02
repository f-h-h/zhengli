<script setup>
// 3D 场景：房间轮廓 + 容器方块渲染 + TransformControls 编辑。
// 桌面：完整交互（移动/旋转/缩放）；移动端：只读预览（OrbitControls 旋转查看）。
import { ref, onMounted, onBeforeUnmount, watch } from 'vue'
import * as THREE from 'three'
import { OrbitControls } from 'three/examples/jsm/controls/OrbitControls.js'
import { TransformControls } from 'three/examples/jsm/controls/TransformControls.js'
import { RoundedBoxGeometry } from 'three/examples/jsm/geometries/RoundedBoxGeometry.js'
import { store, toast, currentRoom, selectedLoc, reloadLocations, locById } from '../store'
import { typeColor, TRANSPARENT_TYPES, typeLabel } from '../types'
import { api } from '../api'

const container = ref(null)
const mode = ref('translate') // translate | rotate | scale
const hint = ref('')
let renderer, scene, camera, orbit, tcontrols, raycaster
let roomGroup, locGroup
const meshes = new Map() // location id → mesh
let rafId = 0
let ro = null
let renderDirty = true // 按需渲染：仅在场景变更时重绘

const SEL_COLOR = 0x4f8cff

// 标记需要重绘（相机阻尼期间 OrbitControls 每帧派发 change 自动保持）。
function markDirty() { renderDirty = true }

// 释放 group 内几何与材质，重建前调用防止 GPU 内存累积。
function disposeGroup(g) {
  g?.traverse(o => {
    if (o.geometry) o.geometry.dispose()
    if (o.material) {
      const mats = Array.isArray(o.material) ? o.material : [o.material]
      for (const m of mats) m.dispose()
    }
  })
}

onMounted(() => {
  init()
  animate()
  ro = new ResizeObserver(() => resize())
  ro.observe(container.value)
})

onBeforeUnmount(() => {
  cancelAnimationFrame(rafId)
  ro?.disconnect()
  tcontrols?.dispose()
  orbit?.dispose()
  renderer?.dispose()
})

watch(() => store.locations, rebuild, { deep: false })
watch(() => store.currentRoomId, rebuildRoom)
// 房间尺寸编辑后 store.rooms 整体重拉，需同步重建墙体轮廓。
watch(() => store.rooms, rebuildRoom)
watch(() => store.selectedId, onSelectChange)
watch(() => store.flashTarget, id => { if (id) tryFlash(id) })
watch(mode, m => { if (tcontrols) tcontrols.setMode(m) })

function init() {
  scene = new THREE.Scene()
  scene.background = new THREE.Color(0xe9edf3)

  camera = new THREE.PerspectiveCamera(50, 1, 0.05, 200)

  renderer = new THREE.WebGLRenderer({ antialias: true })
  renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2))
  // 视觉：软阴影 + 电影级色调映射，避免"塑料方块"感。
  renderer.shadowMap.enabled = true
  renderer.shadowMap.type = THREE.PCFSoftShadowMap
  renderer.toneMapping = THREE.ACESFilmicToneMapping
  renderer.toneMappingExposure = 1.05
  container.value.appendChild(renderer.domElement)

  // 光照：浅色场景需要充足环境光避免物体背光面发灰。
  const amb = new THREE.AmbientLight(0xffffff, 1.0)
  const dir = new THREE.DirectionalLight(0xffffff, 1.1)
  dir.position.set(5, 10, 3)
  dir.castShadow = true
  dir.shadow.mapSize.set(1024, 1024)
  dir.shadow.camera.left = -8
  dir.shadow.camera.right = 8
  dir.shadow.camera.top = 8
  dir.shadow.camera.bottom = -8
  dir.shadow.camera.near = 0.5
  dir.shadow.camera.far = 40
  // 补一个反向弱光，减少死黑背光面。
  const fill = new THREE.DirectionalLight(0xdfe8ff, 0.35)
  fill.position.set(-6, 4, -5)
  scene.add(amb, dir, fill)

  // 地面参考网格。
  const grid = new THREE.GridHelper(12, 12, 0xb8c2d2, 0xd8dfe9)
  grid.position.y = -0.002
  scene.add(grid)

  roomGroup = new THREE.Group()
  locGroup = new THREE.Group()
  scene.add(roomGroup, locGroup)

  orbit = new OrbitControls(camera, renderer.domElement)
  orbit.enableDamping = true
  orbit.maxPolarAngle = Math.PI / 2.05
  // 相机/阻尼动画期间派发 change → 触发按需重绘。
  orbit.addEventListener('change', markDirty)

  // 变换控制器：桌面启用，移动端只读。
  // three r169+：TransformControls 不再是 Object3D，需 getHelper() 加入场景。
  tcontrols = new TransformControls(camera, renderer.domElement)
  tcontrols.setSpace('local')
  tcontrols.setTranslationSnap(0.01)
  tcontrols.setRotationSnap(THREE.MathUtils.degToRad(1))
  tcontrols.addEventListener('dragging-changed', e => {
    orbit.enabled = !e.value
    if (e.value) beginDrag()
    else { onDragEnd(); dragStarts = null }
  })
  // 拖拽实时处理：组内联动 + 近墙吸附 + 墙体钳制（仅 UI 辅助，不拦截保存）。
  tcontrols.addEventListener('objectChange', handleDragFrame)
  if (!store.isMobile) scene.add(tcontrols.getHelper())

  raycaster = new THREE.Raycaster()

  renderer.domElement.addEventListener('pointerdown', onPointerDown)

  resize()
  rebuildRoom()
  rebuild()
}

function resize() {
  if (!container.value) return
  const w = container.value.clientWidth
  const h = container.value.clientHeight
  if (!w || !h) return
  camera.aspect = w / h
  camera.updateProjectionMatrix()
  renderer.setSize(w, h)
  markDirty()
}

function rebuildRoom() {
  disposeGroup(roomGroup)
  roomGroup.clear()
  markDirty()
  const room = currentRoom()
  if (!room) return
  const W = room.room_w || 4, D = room.room_d || 3, H = room.room_h || 2.8

  // 空心墙体轮廓：底面矩形 + 四角竖线 + 顶面矩形。
  const pts = []
  const corners = [
    [0, 0, 0], [W, 0, 0], [W, 0, D], [0, 0, D],
  ]
  // 底面。
  for (let i = 0; i < 4; i++) {
    const [x1, , z1] = corners[i]
    const [x2, , z2] = corners[(i + 1) % 4]
    pts.push(new THREE.Vector3(x1, 0, z1), new THREE.Vector3(x2, 0, z2))
  }
  // 竖边 + 顶面。
  for (let i = 0; i < 4; i++) {
    const [x, , z] = corners[i]
    const [x2, , z2] = corners[(i + 1) % 4]
    pts.push(new THREE.Vector3(x, 0, z), new THREE.Vector3(x, H, z))
    pts.push(new THREE.Vector3(x, H, z), new THREE.Vector3(x2, H, z2))
  }
  const geo = new THREE.BufferGeometry().setFromPoints(pts)
  const line = new THREE.LineSegments(geo, new THREE.LineBasicMaterial({ color: 0x5b6b8c }))
  roomGroup.add(line)

  // 地板。
  const floor = new THREE.Mesh(
    new THREE.PlaneGeometry(W, D),
    new THREE.MeshStandardMaterial({ color: 0xffffff, roughness: 0.95 })
  )
  floor.rotation.x = -Math.PI / 2
  floor.position.set(W / 2, -0.005, D / 2)
  floor.receiveShadow = true
  roomGroup.add(floor)

  // 相机机位面向房间中心。
  const cx = W / 2, cz = D / 2
  camera.position.set(cx + W * 0.9, Math.max(H * 1.2, 3), cz + D * 1.4)
  orbit.target.set(cx, 0.8, cz)
  orbit.update()
}

// 圆角盒：半径取最小边的 12%，柔化轮廓。
function roundedBox(w, h, d, mat) {
  const r = Math.min(w, h, d) * 0.12
  const seg = 2
  // RoundedBoxGeometry 参数：宽高深、分段、半径。
  return new THREE.Mesh(new RoundedBoxGeometry(Math.max(w, 0.01), Math.max(h, 0.01), Math.max(d, 0.01), seg, r), mat)
}

// 按类型构建复合模型：返回 Group（原点=逻辑包围盒中心），子网格相对中心布置。
// 数据量小（家用几十个），每次全量重建可接受。
function buildVisual(l, mat) {
  const g = new THREE.Group()
  const w = Math.max(l.w || 0.2, 0.02), h = Math.max(l.h || 0.2, 0.02), d = Math.max(l.d || 0.2, 0.02)
  const add = m => { m.castShadow = !TRANSPARENT_TYPES.has(l.location_type); m.receiveShadow = true; g.add(m) }

  switch (l.location_type) {
    case 'table': {
      // 台面：顶板 + 四腿（腿高占比 82%）。
      const topT = Math.min(0.05, h * 0.18)
      const top = roundedBox(w, topT, d, mat)
      top.position.y = h / 2 - topT / 2
      add(top)
      const legT = Math.min(w, d) * 0.12
      const legH = h - topT
      for (const [sx, sz] of [[1, 1], [1, -1], [-1, 1], [-1, -1]]) {
        const leg = new THREE.Mesh(new THREE.BoxGeometry(legT, legH, legT), mat)
        leg.position.set(sx * (w / 2 - legT), -topT / 2, sz * (d / 2 - legT))
        add(leg)
      }
      break
    }
    case 'door': {
      // 门板 + 把手小球（朝 +Z 侧偏移）。
      const panel = roundedBox(w, h, d, mat)
      add(panel)
      const knob = new THREE.Mesh(
        new THREE.SphereGeometry(Math.min(w, h) * 0.045, 12, 10),
        new THREE.MeshStandardMaterial({ color: 0xb8b8c0, metalness: 0.7, roughness: 0.3 })
      )
      knob.position.set(w * 0.38, 0, d / 2 + d * 0.9)
      add(knob)
      break
    }
    case 'window': {
      // 窗框 + 玻璃板（玻璃另用高透材质）。
      const ft = Math.min(w, h) * 0.07
      for (const [sw, sh, px, py] of [
        [w, ft, 0, h / 2 - ft / 2], [w, ft, 0, -h / 2 + ft / 2],
        [ft, h - ft * 2, -w / 2 + ft / 2, 0], [ft, h - ft * 2, w / 2 - ft / 2, 0],
      ]) {
        const bar = new THREE.Mesh(new THREE.BoxGeometry(sw, sh, d), mat)
        bar.position.set(px, py, 0)
        add(bar)
      }
      const glass = new THREE.Mesh(
        new THREE.BoxGeometry(w - ft * 2, h - ft * 2, d * 0.3),
        new THREE.MeshPhysicalMaterial({
          color: 0xcfe8ff, transparent: true, opacity: 0.35,
          roughness: 0.05, metalness: 0, transmission: 0.6,
        })
      )
      add(glass)
      break
    }
    case 'hanger': {
      // 落地衣架：底盘 + 立杆 + 顶部挂杆。
      const base = new THREE.Mesh(new THREE.CylinderGeometry(w * 0.28, w * 0.32, 0.03, 16), mat)
      base.position.y = -h / 2 + 0.015
      add(base)
      const pole = new THREE.Mesh(new THREE.CylinderGeometry(w * 0.035, w * 0.035, h - 0.06, 10), mat)
      pole.position.y = 0
      add(pole)
      const bar = new THREE.Mesh(new THREE.CylinderGeometry(w * 0.025, w * 0.025, w * 0.9, 10), mat)
      bar.rotation.z = Math.PI / 2
      bar.position.y = h / 2 - 0.03
      add(bar)
      break
    }
    case 'luggage': {
      // 行李箱：圆角箱体 + 拉杆 + 顶部提手。
      add(roundedBox(w, h, d, mat))
      const rodR = Math.min(w, h) * 0.03
      for (const sx of [-1, 1]) {
        const rod = new THREE.Mesh(
          new THREE.CylinderGeometry(rodR, rodR, h * 0.5, 8),
          new THREE.MeshStandardMaterial({ color: 0x8a8f99, metalness: 0.6, roughness: 0.35 })
        )
        rod.position.set(sx * w * 0.25, h / 2 + h * 0.25, 0)
        add(rod)
      }
      const handle = new THREE.Mesh(
        new THREE.BoxGeometry(w * 0.55, rodR * 2, rodR * 2),
        new THREE.MeshStandardMaterial({ color: 0x8a8f99, metalness: 0.6, roughness: 0.35 })
      )
      handle.position.set(0, h / 2 + h * 0.5, 0)
      add(handle)
      break
    }
    case 'wall_mount': {
      // 挂钩/挂篮：薄背板 + 前伸托板。
      const back = new THREE.Mesh(new THREE.BoxGeometry(w, h, Math.max(d * 0.2, 0.01)), mat)
      add(back)
      const basket = roundedBox(w, h * 0.4, d, mat)
      basket.position.set(0, -h / 2 + h * 0.2, d / 2)
      add(basket)
      break
    }
    case 'drawer': {
      // 抽屉：柜体 + 前面板外凸 + 拉手，视觉上读作"抽屉"。
      // 真实抽屉与柜内隔间共用此类型（规格 §2）；面板朝 +Z（局部）。
      add(roundedBox(w, h, d, mat))
      const faceT = Math.max(d * 0.08, 0.015)
      const face = roundedBox(w * 0.96, h * 0.82, faceT, mat)
      face.position.z = d / 2 + faceT / 2 - 0.002
      add(face)
      const knobR = Math.max(Math.min(w, h) * 0.06, 0.008)
      const knob = new THREE.Mesh(
        new THREE.SphereGeometry(knobR, 12, 10),
        new THREE.MeshStandardMaterial({ color: 0xb8b8c0, metalness: 0.7, roughness: 0.3 })
      )
      knob.position.z = d / 2 + faceT + knobR * 0.6
      add(knob)
      break
    }
    case 'shelf_layer': {
      // 层板：薄板（自身 h 就是板厚），不额外加工。
      add(roundedBox(w, h, d, mat))
      break
    }
    default: {
      // 其余类型：圆角盒通用。
      add(roundedBox(w, h, d, mat))
    }
  }
  return g
}

// 全量重建容器方块（数据量小，简单可靠）。
function rebuild() {
  // 保留选中态信息，重建后恢复。
  const selId = store.selectedId
  tcontrols?.detach()
  disposeGroup(locGroup)
  locGroup.clear()
  meshes.clear()
  markDirty()

  for (const l of store.locations) {
    const trans = TRANSPARENT_TYPES.has(l.location_type)
    // 用户未自定义颜色（空或默认灰）时按类型着色。
    const custom = l.color && l.color !== '#cccccc' ? l.color : typeColor(l.location_type)
    const mat = new THREE.MeshStandardMaterial({
      color: new THREE.Color(custom),
      transparent: trans,
      opacity: trans ? 0.35 : 1,
      roughness: 0.75,
    })
    const mesh = buildVisual(l, mat)
    mesh.position.set(l.x, l.y, l.z)
    mesh.rotation.y = l.rot_y || 0
    mesh.userData = { locId: l.id, baseW: l.w, baseH: l.h, baseD: l.d, groupId: l.geom_group || null }
    // 逻辑包围盒描边：选中高亮 / 平时淡描。
    const edge = new THREE.LineSegments(
      new THREE.EdgesGeometry(new THREE.BoxGeometry(l.w || 0.2, l.h || 0.2, l.d || 0.2)),
      new THREE.LineBasicMaterial({ color: l.id === selId ? SEL_COLOR : 0x8a94a6, transparent: true, opacity: l.id === selId ? 0.95 : 0.28 })
    )
    mesh.add(edge)
    mesh.userData.edge = edge
    locGroup.add(mesh)
    meshes.set(l.id, mesh)
  }
  if (selId) applySelection(selId)
  // rebuild 后 meshes 全新：闪烁目标若存在则重新挂载。
  if (store.flashTarget) tryFlash(store.flashTarget)
  // 重建时落地吸附一次：表单/API 新建的悬空实体自动落回地板或支撑面。
  settleAssembly([...meshes.values()])
  hint.value = ''
}

function applySelection(id) {
  const mesh = meshes.get(id)
  if (!mesh) { tcontrols?.detach(); return }
  // 柜内隔间与柜体一体：选中可看详情，但不挂 gizmo（不可单独移动）。
  const loc = locById.get(id)
  if (!loc || !isCompartment(loc)) {
    if (!store.isMobile) tcontrols.attach(mesh)
  } else {
    tcontrols?.detach()
  }
  if (loc) hint.value = `${loc.name}（${typeLabel(loc.location_type)}）`
}

// 柜内隔间（drawer 且有父容器）：与柜体一体，不能单独移动。
function isCompartment(loc) {
  return loc.location_type === 'drawer' && loc.parent_id != null
}

// 拖拽目标重定向：拖隔间 = 拖整个父容器。
function dragTargetFor(mesh) {
  if (!mesh) return mesh
  const loc = locById.get(mesh.userData.locId)
  if (loc && isCompartment(loc)) {
    const pm = meshes.get(loc.parent_id)
    if (pm) return pm
  }
  return mesh
}

// 递归收集全部下级容器的 mesh（拖父容器时整组跟随）。
function descendantMeshes(locId) {
  const out = []
  const walk = pid => {
    store.locations.forEach(l => {
      if (l.parent_id === pid) {
        const m = meshes.get(l.id)
        if (m) out.push(m)
        walk(l.id)
      }
    })
  }
  walk(locId)
  return out
}

function onSelectChange(id) {
  markDirty()
  meshes.forEach(m => {
    m.userData.edge.material.color.set(0x8a94a6)
    m.userData.edge.material.opacity = 0.28
  })
  if (!id) { tcontrols?.detach(); hint.value = ''; return }
  applySelection(id)
  const mesh = meshes.get(id)
  if (mesh) {
    mesh.userData.edge.material.color.set(SEL_COLOR)
    mesh.userData.edge.material.opacity = 0.95
  }
}

let downX = 0, downY = 0
// NDC 坐标换算（事件 → 相机射线参数）。
function ndcOf(ev) {
  const rect = renderer.domElement.getBoundingClientRect()
  return new THREE.Vector2(
    ((ev.clientX - rect.left) / rect.width) * 2 - 1,
    -((ev.clientY - rect.top) / rect.height) * 2 + 1
  )
}
// 拾取：返回命中物体的根组（复合模型沿父链上溯）。
function pickMesh(ev) {
  raycaster.setFromCamera(ndcOf(ev), camera)
  const hits = raycaster.intersectObjects([...meshes.values()], true)
  if (!hits.length) return null
  let o = hits[0].object
  while (o && o.userData?.locId === undefined) o = o.parent
  return o ?? null
}
// 触摸拖拽中：射线与物体等高水平面求交点。
const dragPlane = new THREE.Plane(new THREE.Vector3(0, 1, 0), 0)
function planePoint(ev, y, out) {
  dragPlane.constant = -y
  raycaster.setFromCamera(ndcOf(ev), camera)
  return raycaster.ray.intersectPlane(dragPlane, out)
}

function onPointerDown(e) {
  if (e.button !== 0) return
  // gizmo 操作（箭头/环）优先：TransformControls 先于本监听注册，已置 dragging。
  if (tcontrols?.dragging) return
  downX = e.clientX; downY = e.clientY
  // 直接拖拽：移动端触摸，或桌面 translate 模式按住物体本体。
  // Shift 按下时改为调高度（Y），否则沿地面平面（XZ）拖动。
  const allowDirect = store.isMobile ? e.pointerType === 'touch' : mode.value === 'translate'
  let drag = null
  if (allowDirect) {
    const m = dragTargetFor(pickMesh(e))
    if (m) drag = { mesh: m, moved: false, yMode: e.shiftKey, startY: m.position.y }
  }
  const move = ev => {
    if (!drag) return
    if (!drag.moved) {
      if (Math.abs(ev.clientX - downX) > 6 || Math.abs(ev.clientY - downY) > 6) {
        drag.moved = true
        orbit.enabled = false
        beginDrag(drag.mesh, drag.yMode)
      } else return
    }
    markDirty()
    if (drag.yMode) {
      // 屏幕 y 像素 → 世界 Y：按相机距离与 fov 换算。
      const dist = camera.position.distanceTo(orbit.target)
      const worldPerPx = (2 * dist * Math.tan(camera.fov * Math.PI / 360)) / renderer.domElement.clientHeight
      drag.mesh.position.y = drag.startY + (downY - ev.clientY) * worldPerPx
      applyDragConstraints(drag.mesh, true)
      return
    }
    const pt = planePoint(ev, drag.mesh.position.y, new THREE.Vector3())
    if (pt) {
      drag.mesh.position.x = pt.x
      drag.mesh.position.z = pt.z
      applyDragConstraints(drag.mesh, false)
    }
  }
  const up = ev => {
    window.removeEventListener('pointerup', up)
    window.removeEventListener('pointermove', move)
    if (drag?.moved) {
      orbit.enabled = true
      saveMeshDrag(drag.mesh)
      dragStarts = null
      return
    }
    // 点击（非拖拽）才触发选择。
    if (Math.abs(ev.clientX - downX) > 4 || Math.abs(ev.clientY - downY) > 4) return
    if (tcontrols?.dragging) return
    const m = pickMesh(ev)
    store.selectedId = m?.userData?.locId ?? null
  }
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
}

// 组联动状态：拖拽开始时记录组内各成员的位置快照。
let dragStarts = null
let dragLift = 0 // 拖拽中的视觉抬起量（XZ 平移时 4cm，保存时剔除）

function beginDrag(mesh = tcontrols?.object, yMode = false) {
  dragStarts = null
  if (!mesh) return
  const loc = locById.get(mesh.userData.locId)
  // 成员 = 自身 + geom_group 组员 + 全部下级（柜子拖动时隔间/箱中箱整组跟随）。
  const members = new Set([mesh])
  const gid = loc?.geom_group
  if (gid) meshes.forEach(m => { if (locById.get(m.userData.locId)?.geom_group === gid) members.add(m) })
  if (loc) descendantMeshes(loc.id).forEach(m => members.add(m))
  // 视觉反馈：平移拖拽时整体轻微抬起，松手落回。Shift 调高度不抬。
  dragLift = yMode ? 0 : 0.04
  if (dragLift) members.forEach(m => { m.position.y += dragLift })
  dragStarts = new Map([...members].map(m => [m, {
    x: m.position.x, y: m.position.y, z: m.position.z, rot: m.rotation.y,
  }]))
  resetCollision()
  markDirty()
}

// 组联动 + 墙吸附 + 钳制 + 碰撞（translate 帧处理，桌面 TransformControls 与直接拖拽共用）。
function applyDragConstraints(mesh, yMode = false) {
  if (dragStarts) {
    const st = dragStarts.get(mesh)
    if (st) {
      const dx = mesh.position.x - st.x, dy = mesh.position.y - st.y, dz = mesh.position.z - st.z
      for (const [m, s] of dragStarts) {
        if (m !== mesh) m.position.set(s.x + dx, s.y + dy, s.z + dz)
      }
    }
  }
  snapToWall(mesh)
  // 边对齐吸附：边缘接近墙体/其它实体时自动齐平（随后钳制+碰撞兜底）。
  alignEdges(mesh, yMode)
  clampMesh(mesh)
  if (dragStarts) {
    for (const [m] of dragStarts) if (m !== mesh) clampMesh(m)
  }
  // 物体间碰撞：不与其它实体互相穿透（区域标记类不参与）。
  collideMesh(mesh, yMode)
}

// AABB（含旋转的保守包围盒）。
function aabbOf(mesh) {
  const { baseW, baseH, baseD } = mesh.userData
  const { ex, ez } = extents(baseW, baseD, mesh.rotation.y)
  const p = mesh.position
  return {
    x0: p.x - ex, x1: p.x + ex,
    y0: p.y - baseH / 2, y1: p.y + baseH / 2,
    z0: p.z - ez, z1: p.z + ez,
  }
}
function aabbOverlap(a, b) {
  return a.x0 < b.x1 && a.x1 > b.x0 && a.y0 < b.y1 && a.y1 > b.y0 && a.z0 < b.z1 && a.z1 > b.z0
}

// 拖拽中的物体（含组员）与其它实体碰撞 → 优先"贴面滑动"（沿推出量小的轴滑出），
// 滑不动（多面夹击）才回退到最近一次合法位置。
// 例外：业务祖先/后代（隔间在柜内属正常嵌套）、同 geom_group、区域标记类。
const COLLIDE_SKIP = new Set(['ground_spot', 'obstacle', 'window'])
let lastGoodPos = null // 拖拽会话内最近合法位置快照

// 是否与其它实体相交（含豁免逻辑）；返回相交方集合。
function findColliders(movers, boxes, myLoc) {
  const out = []
  meshes.forEach(other => {
    if (movers.includes(other)) return
    const oLoc = locById.get(other.userData.locId)
    if (!oLoc) return
    if (COLLIDE_SKIP.has(oLoc.location_type)) return
    if (myLoc.geom_group && oLoc.geom_group === myLoc.geom_group) return
    // 业务祖先/后代不算：隔间在柜子里、柜子里放箱子属正常几何嵌套。
    let a = myLoc
    while (a?.parent_id) {
      if (a.parent_id === oLoc.id) return
      a = locById.get(a.parent_id)
    }
    let d = oLoc
    while (d?.parent_id) {
      if (d.parent_id === myLoc.id) return
      d = locById.get(d.parent_id)
    }
    const ob = aabbOf(other)
    if (boxes.some(mb => aabbOverlap(mb, ob))) out.push({ other, ob })
  })
  return out
}

function collideMesh(mesh, yMode = false) {
  const movers = dragStarts ? [...dragStarts.keys()] : [mesh]
  const myLoc = locById.get(mesh.userData.locId)
  if (!myLoc) return
  const boxes = movers.map(aabbOf)
  let hit = findColliders(movers, boxes, myLoc)
  if (!hit.length) {
    lastGoodPos = movers.map(m => [m, m.position.clone()])
    return
  }
  // 贴面滑动：对主 mesh 计算推出向量（每个相交方选重叠量小的轴、推出方向按中心侧）。
  if (!yMode) {
    let px = 0, pz = 0
    for (const { ob } of hit) {
      const mb = aabbOf(mesh)
      const ox = Math.min(mb.x1 - ob.x0, ob.x1 - mb.x0)
      const oz = Math.min(mb.z1 - ob.z0, ob.z1 - mb.z0)
      if (ox <= oz) {
        const dir = mesh.position.x < (ob.x0 + ob.x1) / 2 ? -1 : 1
        const need = dir > 0 ? ob.x1 - mb.x0 : mb.x1 - ob.x0
        px = dir > 0 ? Math.max(px, need) : Math.min(px, -need)
      } else {
        const dir = mesh.position.z < (ob.z0 + ob.z1) / 2 ? -1 : 1
        const need = dir > 0 ? ob.z1 - mb.z0 : mb.z1 - ob.z0
        pz = dir > 0 ? Math.max(pz, need) : Math.min(pz, -need)
      }
    }
    if (px || pz) {
      // 整组平移推出量，再校验一次；仍有相交（夹缝放不下）则放弃滑动。
      movers.forEach(m => { m.position.x += px; m.position.z += pz })
      clampMesh(mesh)
      movers.forEach(clampMesh)
      const still = findColliders(movers, movers.map(aabbOf), myLoc)
      if (!still.length) {
        lastGoodPos = movers.map(m => [m, m.position.clone()])
        return
      }
    }
  } else {
    // Y 模式：垂直推出（箱底不能压进下方箱顶）。
    let py = 0
    for (const { ob } of hit) {
      const mb = aabbOf(mesh)
      const dir = mesh.position.y < (ob.y0 + ob.y1) / 2 ? -1 : 1
      const need = dir > 0 ? ob.y1 - mb.y0 : mb.y1 - ob.y0
      py = dir > 0 ? Math.max(py, need) : Math.min(py, -need)
    }
    if (py) {
      movers.forEach(m => { m.position.y += py })
      const still = findColliders(movers, movers.map(aabbOf), myLoc)
      if (!still.length) {
        lastGoodPos = movers.map(m => [m, m.position.clone()])
        return
      }
    }
  }
  // 滑不动：回退到最近合法位置。
  if (lastGoodPos) for (const [m, p] of lastGoodPos) m.position.copy(p)
  else if (dragStarts) for (const [m, s] of dragStarts) m.position.set(s.x, s.y, s.z)
}

// beginDrag 时重置碰撞基线。
function resetCollision() {
  const movers = dragStarts ? [...dragStarts.keys()] : []
  lastGoodPos = movers.map(m => [m, m.position.clone()])
}

// 每帧拖拽（桌面 TransformControls）。
function handleDragFrame() {
  markDirty()
  const mesh = tcontrols?.object
  if (!mesh) return
  if (mode.value === 'translate' && dragStarts) {
    // gizmo 拖 Y 轴箭头时走垂直碰撞（推出方向为上下）。
    applyDragConstraints(mesh, tcontrols.axis === 'Y')
  } else clampMesh(mesh)
}

// 近墙吸附：门/窗/挂墙物拖到墙边时自动贴墙并转向（用户松手前可见效果）。
const SNAP_DIST = 0.3
const WALL_TYPES = new Set(['door', 'window', 'wall_mount'])
function snapToWall(mesh) {
  const loc = locById.get(mesh.userData.locId)
  if (!loc || !WALL_TYPES.has(loc.location_type)) return
  const room = currentRoom()
  if (!room) return
  const W = room.room_w || 4, D = room.room_d || 3
  const { baseD } = mesh.userData
  // 吸附后所有朝向的法向厚度都是 d（东西墙转 90° 后深度沿法向）。
  const t = baseD / 2
  // 各墙：北 z=0 → rot 0；南 z=D → π；西 x=0 → π/2；东 x=W → -π/2。
  const gaps = [
    { gap: mesh.position.z - t, apply: () => { mesh.position.z = t; mesh.rotation.y = 0 } },
    { gap: D - mesh.position.z - t, apply: () => { mesh.position.z = D - t; mesh.rotation.y = Math.PI } },
    { gap: mesh.position.x - t, apply: () => { mesh.position.x = t; mesh.rotation.y = Math.PI / 2 } },
    { gap: W - mesh.position.x - t, apply: () => { mesh.position.x = W - t; mesh.rotation.y = -Math.PI / 2 } },
  ]
  const best = gaps.reduce((a, b) => (b.gap < a.gap ? b : a))
  if (best.gap > SNAP_DIST) return
  best.apply()
}

// 边对齐吸附：拖拽中物体的边缘接近墙体或其它实体边缘（容差内）时自动对齐齐平。
// 参照 = 房间四面墙 + 可对齐实体；仅在 XZ 平移时触发（Shift 调高 Y 不参与），
// 整组（geom_group/下级）同步平移保持相对位置。
const ALIGN_DIST = 0.08 // 吸附容差（米）
const ALIGN_OVERLAP = 0.05 // 有效对齐所需的最小共面重叠（米）
// 不作为对齐参照的类型：区域标记/固定件（贴墙物走 snapToWall 逻辑）。
const ALIGN_SKIP = new Set(['ground_spot', 'obstacle', 'window', 'door', 'wall_mount'])
function alignEdges(mesh, yMode) {
  if (yMode) return
  const myLoc = locById.get(mesh.userData.locId)
  if (!myLoc) return
  const { baseW, baseD } = mesh.userData
  const { ex, ez } = extents(baseW, baseD, mesh.rotation.y)
  const p = mesh.position
  // 候选参照边：axis=对齐轴，v=边所在位置，c0/c1=垂直方向跨度。
  const cands = []
  const room = currentRoom()
  if (room) {
    const W = room.room_w || 4, D = room.room_d || 3
    cands.push({ axis: 'x', v: 0, c0: -1e4, c1: 1e4 })
    cands.push({ axis: 'x', v: W, c0: -1e4, c1: 1e4 })
    cands.push({ axis: 'z', v: 0, c0: -1e4, c1: 1e4 })
    cands.push({ axis: 'z', v: D, c0: -1e4, c1: 1e4 })
  }
  meshes.forEach(other => {
    if (other === mesh) return
    const oLoc = locById.get(other.userData.locId)
    if (!oLoc) return
    if (ALIGN_SKIP.has(oLoc.location_type)) return
    if (myLoc.geom_group && oLoc.geom_group === myLoc.geom_group) return
    if (isRelated(myLoc, oLoc)) return
    const ob = aabbOf(other)
    cands.push({ axis: 'x', v: ob.x0, c0: ob.z0, c1: ob.z1 })
    cands.push({ axis: 'x', v: ob.x1, c0: ob.z0, c1: ob.z1 })
    cands.push({ axis: 'z', v: ob.z0, c0: ob.x0, c1: ob.x1 })
    cands.push({ axis: 'z', v: ob.z1, c0: ob.x0, c1: ob.x1 })
  })
  if (!cands.length) return
  // 每轴取最近的吸附；垂直方向需有足够共面重叠，防止无关边误吸。
  let bestX = null, bestZ = null
  for (const c of cands) {
    if (c.axis === 'x') {
      if (Math.max(p.z - ez, c.c0) > Math.min(p.z + ez, c.c1) - ALIGN_OVERLAP) continue
      for (const e of [p.x - ex, p.x + ex]) {
        const gap = Math.abs(e - c.v)
        if (gap < ALIGN_DIST && (!bestX || gap < bestX.gap)) bestX = { gap, d: c.v - e }
      }
    } else {
      if (Math.max(p.x - ex, c.c0) > Math.min(p.x + ex, c.c1) - ALIGN_OVERLAP) continue
      for (const e of [p.z - ez, p.z + ez]) {
        const gap = Math.abs(e - c.v)
        if (gap < ALIGN_DIST && (!bestZ || gap < bestZ.gap)) bestZ = { gap, d: c.v - e }
      }
    }
  }
  const dx = bestX ? bestX.d : 0
  const dz = bestZ ? bestZ.d : 0
  if (!dx && !dz) return
  if (dragStarts) for (const [m] of dragStarts) { m.position.x += dx; m.position.z += dz }
  else { mesh.position.x += dx; mesh.position.z += dz }
}

// 单个 mesh 的墙体钳制：中心保持在房间内、底面不入地板。
function clampMesh(mesh) {
  const room = currentRoom()
  if (!room) return
  const { baseW, baseH, baseD } = mesh.userData
  const { ex, ez } = extents(baseW, baseD, mesh.rotation.y)
  const W = room.room_w || 4, D = room.room_d || 3
  // 超大物体取半房，避免 clamp 区间反转。
  const hx = Math.min(ex, W / 2), hz = Math.min(ez, D / 2)
  mesh.position.x = THREE.MathUtils.clamp(mesh.position.x, hx, W - hx)
  mesh.position.z = THREE.MathUtils.clamp(mesh.position.z, hz, D - hz)
  mesh.position.y = Math.max(mesh.position.y, baseH / 2)
}

// 落地吸附：拖拽松手时，允许落地的顶层实体整体下沉到下方支撑面（地板或另一物体顶面）。
// 嵌套容器（抽屉/隔间/柜内物品）与固定件（门/窗/挂墙物/障碍物/区域标记）保持原位，
// 几何联动组成员跟随整体平移，保证柜内/架上的相对位置不被打散。
const SETTLE_SKIP = new Set(['obstacle', 'ground_spot', 'window', 'door', 'wall_mount'])

function isSettleable(loc) {
  if (!loc) return false
  if (loc.parent_id != null) return false   // 嵌套容器：由父容器定位
  if (SETTLE_SKIP.has(loc.location_type)) return false
  return true
}

// a/b 是否属性归属相关（祖先/后代/同几何组），相关则彼此不互为支撑也不碰撞。
function isRelated(a, b) {
  if (a.geom_group && b.geom_group === a.geom_group) return true
  let x = a
  while (x?.parent_id) {
    if (x.parent_id === b.id) return true
    x = locById.get(x.parent_id)
  }
  let y = b
  while (y?.parent_id) {
    if (y.parent_id === a.id) return true
    y = locById.get(y.parent_id)
  }
  return false
}

// 返回 mesh 下方（同足迹内、顶面在其底以下）的最高支撑面高度，地板为 0；无则地板。
function supportBelow(m) {
  let top = 0
  const mb = aabbOf(m)
  meshes.forEach(other => {
    if (other === m) return
    const oLoc = locById.get(other.userData.locId)
    if (!oLoc) return
    if (SETTLE_SKIP.has(oLoc.location_type)) return
    const myLoc = locById.get(m.userData.locId)
    if (myLoc && isRelated(myLoc, oLoc)) return
    const ob = aabbOf(other)
    if (ob.y1 <= mb.y0 + 0.012 && mb.x0 < ob.x1 && mb.x1 > ob.x0 && mb.z0 < ob.z1 && mb.z1 > ob.z0) {
      if (ob.y1 > top) top = ob.y1
    }
  })
  return top
}

// 整组下沉：取所有可落地成员的最小"离地间隙"，整体平移该距离（保持组内相对位置）。
function settleAssembly(targets) {
  markDirty()
  let drop = 0
  for (const m of targets) {
    const loc = locById.get(m.userData.locId)
    if (!isSettleable(loc)) continue
    const h = m.userData.baseH
    const bottom = m.position.y - h / 2
    const gap = bottom - supportBelow(m)
    if (gap > 0.006 && (drop === 0 || gap < drop)) drop = gap
  }
  if (drop > 0.006) targets.forEach(m => { m.position.y -= drop })
}

// 保存拖拽结果（组联动时保存全部成员）。translate/触摸拖拽共用。
// 保存前剔除视觉抬起量（dragLift）并落地吸附，再写回真实坐标。
async function saveMeshDrag(mesh) {
  markDirty()
  const targets = dragStarts ? [...dragStarts.keys()] : [mesh]
  try {
    if (dragLift) {
      targets.forEach(m => { m.position.y -= dragLift })
      dragLift = 0
    }
    settleAssembly(targets)
    for (const m of targets) {
      const ud = m.userData
      const g = {
        x: +m.position.x.toFixed(4), y: +m.position.y.toFixed(4), z: +m.position.z.toFixed(4),
        w: ud.baseW, h: ud.baseH, d: ud.baseD, rot_y: +m.rotation.y.toFixed(4),
      }
      await api.patchGeom(ud.locId, g)
      // 本地同步（不重载，避免闪烁）。
      const loc = locById.get(ud.locId)
      if (loc) { loc.x = g.x; loc.y = g.y; loc.z = g.z; loc.rot_y = g.rot_y }
    }
    const { baseW, baseH, baseD } = mesh.userData
    checkWarnings(mesh.position.x, mesh.position.y, mesh.position.z, baseW, baseH, baseD, mesh.rotation.y)
  } catch (e) {
    toast(e.message, 'err')
  }
}

// 拖拽结束：保存几何（组联动时保存全部成员）+ 警告提示（仅 UI 提示，不拦截保存，需求 §5-5）。
async function onDragEnd() {
  const mesh = tcontrols?.object
  if (!mesh) { dragStarts = null; return }
  const { locId, baseW, baseH, baseD } = mesh.userData
  const p = mesh.position, r = mesh.rotation

  // scale 模式：把缩放烘焙回尺寸，避免网格变形累积。
  if (mode.value === 'scale') {
    const s = mesh.scale
    const w = +(baseW * s.x).toFixed(4), h = +(baseH * s.y).toFixed(4), d = +(baseD * s.z).toFixed(4)
    const loc = locById.get(locId)
    if (loc) {
      loc.w = w; loc.h = h; loc.d = d
      loc.x = +p.x.toFixed(4); loc.y = +p.y.toFixed(4); loc.z = +p.z.toFixed(4)
      loc.rot_y = +r.toFixed(4)
    }
    rebuild()
    applySelection(store.selectedId)
    dragStarts = null
    return
  }

  // gizmo 拖拽（transformControl）也要落地吸附。
  const targets = dragStarts ? [...dragStarts.keys()] : [mesh]
  if (dragLift) {
    targets.forEach(m => { m.position.y -= dragLift })
    dragLift = 0
  }
  settleAssembly(targets)
  await saveMeshDrag(mesh)
}

// 旋转后 AABB 半径。
function extents(w, d, rot) {
  const c = Math.abs(Math.cos(rot)), s = Math.abs(Math.sin(rot))
  return { ex: c * w / 2 + s * d / 2, ez: s * w / 2 + c * d / 2 }
}

function checkWarnings(x, y, z, w, h, d, rot) {
  const locs = store.locations
  const obstacles = locs.filter(l => l.location_type === 'obstacle')
  const spots = locs.filter(l => l.location_type === 'ground_spot')
  const me = extents(w, d, rot)

  // 与 obstacle 重叠 → 黄色警告。
  for (const ob of obstacles) {
    const oe = extents(ob.w, ob.d, ob.rot_y)
    if (
      Math.abs(x - ob.x) < me.ex + oe.ex &&
      Math.abs(y - ob.y) < (h + ob.h) / 2 &&
      Math.abs(z - ob.z) < me.ez + oe.ez
    ) {
      toast('与障碍物空间重叠', 'warn')
      return
    }
  }

  // 不在任何可用区域内 → 灰色弱提示。
  if (spots.length) {
    const inside = spots.some(sp => {
      const se = extents(sp.w, sp.d, sp.rot_y)
      return (
        Math.abs(x - sp.x) < se.ex && Math.abs(z - sp.z) < se.ez &&
        y - h / 2 >= (sp.y - sp.h / 2) - 0.02
      )
    })
    if (!inside) toast('该位置未标记可用空间', 'info')
  }
}

function animate() {
  rafId = requestAnimationFrame(animate)
  orbit.update()
  tickFlash()
  // 按需渲染：相机静止且无闪烁时不重绘，降低 GPU 占用。
  if (renderDirty || flash) {
    renderer.render(scene, camera)
    renderDirty = false
  }
}

// ===== 寻物定位闪烁 =====
// 直接容器呼吸脉冲（缩放+描边闪烁），祖先链橙色描边（柜子→箱子逐级指路）。
const FLASH_MS = 3000
const FLASH_ANCESTOR = 0xff9800
let flash = null // { mesh, ancestors:[mesh], until }

function tryFlash(id, retries = 20) {
  const mesh = meshes.get(id)
  // 视图切换/房间加载可能晚于 watch 触发，mesh 未就绪时重试。
  if (!mesh) {
    if (retries > 0) setTimeout(() => tryFlash(id, retries - 1), 100)
    return
  }
  markDirty()
  // 祖先链：沿业务归属 parent_id 上溯（需求 §5-3：归属不靠几何推导）。
  const ancestors = []
  let cur = locById.get(id)
  while (cur?.parent_id) {
    const p = locById.get(cur.parent_id)
    if (!p) break
    const m = meshes.get(p.id)
    if (m) ancestors.push(m)
    cur = p
  }
  if (flash) endFlash()
  flash = { mesh, ancestors, until: performance.now() + FLASH_MS }
  for (const m of ancestors) {
    m.userData.edge.material.color.set(FLASH_ANCESTOR)
    m.userData.edge.material.opacity = 0.9
  }
  // 相机对准目标。
  orbit.target.copy(mesh.position)
}

function endFlash() {
  if (!flash) return
  markDirty()
  flash.mesh.scale.setScalar(1)
  // 恢复描边默认色（选中态由 onSelectChange 重新应用）。
  for (const m of [...flash.ancestors, flash.mesh]) {
    m.userData.edge.material.color.set(0x8a94a6)
    m.userData.edge.material.opacity = 0.28
  }
  flash = null
  store.flashTarget = null
  if (store.selectedId) onSelectChange(store.selectedId)
}

function tickFlash() {
  if (!flash) return
  if (performance.now() > flash.until) { endFlash(); return }
  // 呼吸脉冲：缩放 1→1.12，描边透明度同步。
  const k = 0.5 + 0.5 * Math.sin(performance.now() / 1000 * 2 * Math.PI * 1.6)
  flash.mesh.scale.setScalar(1 + 0.12 * k)
  flash.mesh.userData.edge.material.color.set(SEL_COLOR)
  flash.mesh.userData.edge.material.opacity = 0.3 + 0.7 * k
}
</script>

<template>
  <div ref="container" class="scene"></div>
  <div v-if="!store.isMobile" class="toolbar">
    <button :class="{ primary: mode === 'translate' }" @click="mode = 'translate'">移动</button>
    <button :class="{ primary: mode === 'rotate' }" @click="mode = 'rotate'">旋转</button>
    <button :class="{ primary: mode === 'scale' }" @click="mode = 'scale'">缩放</button>
    <span class="tip">按住物体直接拖（Shift 拖=调高度）· 点击选中 · 右键平移视角</span>
  </div>
  <div v-if="hint" class="sel-hint">{{ hint }}</div>
  <div v-if="store.isMobile" class="mobile-tip">单指拖动物体 · 空白处旋转视角 · 双指缩放</div>
</template>

<style scoped>
.scene { position: absolute; inset: 0; }
.toolbar {
  position: absolute;
  top: 10px; left: 10px;
  display: flex;
  gap: 6px;
  align-items: center;
  background: rgba(255, 255, 255, 0.92);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 6px 8px;
  z-index: 5;
  box-shadow: 0 2px 8px rgba(30, 40, 60, .08);
}
.tip { font-size: 11px; color: var(--fg-dim); margin-left: 6px; }
.sel-hint {
  position: absolute;
  top: 10px; right: 10px;
  background: rgba(255, 255, 255, 0.92);
  border: 1px solid var(--accent);
  border-radius: var(--radius);
  padding: 6px 12px;
  font-size: 13px;
  color: var(--fg);
  z-index: 5;
}
.mobile-tip {
  position: absolute;
  bottom: 12px; left: 50%;
  transform: translateX(-50%);
  background: rgba(255, 255, 255, 0.92);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 4px 12px;
  font-size: 11px;
  color: var(--fg-dim);
  z-index: 5;
}
</style>
