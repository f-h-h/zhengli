// location_type 元数据：与后端权限矩阵保持一致（需求 §2）。
// 颜色为浅色 3D 场景设计：中高饱和度、白底可辨。
export const TYPE_META = {
  cabinet:     { label: '柜子',       color: '#a4713c', allowChild: true,  allowItem: true },
  drawer:      { label: '抽屉/隔间', color: '#c68a52', allowChild: true,  allowItem: true },
  shelf_layer: { label: '置物板',     color: '#9aa5b1', allowChild: true,  allowItem: true },
  box:         { label: '收纳箱',     color: '#3d8bfd', allowChild: true,  allowItem: true },
  cardbox:     { label: '纸箱',       color: '#d9a441', allowChild: true,  allowItem: true },
  luggage:     { label: '行李箱',     color: '#a855f7', allowChild: true,  allowItem: true },
  wall_mount:  { label: '挂墙收纳',   color: '#14b8a6', allowChild: true,  allowItem: true },
  table:       { label: '台面',       color: '#e0a458', allowChild: true,  allowItem: true },
  hanger:      { label: '落地衣架',   color: '#84cc16', allowChild: false, allowItem: true },
  tray:        { label: '活动托盘',   color: '#f7c948', allowChild: false, allowItem: false },
  ground_spot: { label: '可用地面',   color: '#4ade80', allowChild: false, allowItem: true },
  furniture:   { label: '床/家具',    color: '#94a3b8', allowChild: false, allowItem: false },
  obstacle:    { label: '障碍物',     color: '#64748b', allowChild: false, allowItem: false },
  door:        { label: '门',         color: '#8a5a2b', allowChild: false, allowItem: false },
  window:      { label: '窗户',       color: '#7db8e8', allowChild: false, allowItem: false },
}

export const TYPE_KEYS = Object.keys(TYPE_META)

export function typeLabel(t) { return TYPE_META[t]?.label ?? t }
export function typeColor(t) { return TYPE_META[t]?.color ?? '#cccccc' }

// 半透明渲染的类型。
// 需要半透明渲染的类型：标记区域 / 障碍 / 玻璃 / 柜体（透出内部隔板与箱子）。
export const TRANSPARENT_TYPES = new Set(['ground_spot', 'obstacle', 'window', 'cabinet'])
