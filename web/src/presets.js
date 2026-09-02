// 基础模型库：常见家具/收纳品的预设模板（单位：米）。
// 仅预填创建表单的默认值，落库后就是普通 location，无特殊地位。
// wall=true 的预设默认贴前墙（z=0）摆放；y 为默认中心高度。

export const PRESET_GROUPS = [
  {
    title: '家具',
    items: [
      { key: 'bed', icon: '🛏️', name: '床', type: 'furniture', w: 1.5, h: 0.5, d: 2.0, color: '#b3c0cf' },
      { key: 'chair', icon: '🪑', name: '椅子', type: 'furniture', w: 0.45, h: 0.9, d: 0.5, color: '#c9a87c' },
      { key: 'desk', icon: 'htable', name: '书桌', type: 'table', w: 1.2, h: 0.75, d: 0.6 },
      { key: 'nightstand', icon: '🗄️', name: '床头柜', type: 'table', special: 'nightstand', w: 0.5, h: 0.55, d: 0.4 },
      { key: 'wardrobe', icon: '👗', name: '衣柜', type: 'cabinet', w: 1.2, h: 2.0, d: 0.6 },
      { key: 'shelf', icon: '📚', name: '置物架', type: 'furniture', special: 'shelf', w: 0.8, h: 1.6, d: 0.3, layers: 4 },
    ],
  },
  {
    title: '收纳',
    items: [
      { key: 'box', icon: '🧰', name: '收纳箱', type: 'box', w: 0.4, h: 0.3, d: 0.3 },
      { key: 'cardbox', icon: '📦', name: '纸箱', type: 'cardbox', w: 0.5, h: 0.4, d: 0.4 },
      { key: 'luggage', icon: '🧳', name: '行李箱', type: 'luggage', w: 0.43, h: 0.65, d: 0.26 },
      { key: 'bag', icon: '👝', name: '收纳袋', type: 'box', w: 0.4, h: 0.5, d: 0.3, color: '#c084fc' },
      { key: 'hanger', icon: '🧥', name: '落地衣架', type: 'hanger', w: 0.5, h: 1.7, d: 0.5 },
    ],
  },
  {
    title: '墙面',
    items: [
      { key: 'door', icon: '🚪', name: '门', type: 'door', w: 0.9, h: 2.0, d: 0.08, wall: true },
      { key: 'window', icon: '🪟', name: '窗户', type: 'window', w: 1.2, h: 1.4, d: 0.08, wall: true, y: 1.5 },
      { key: 'ac', icon: '❄️', name: '壁挂空调', type: 'obstacle', w: 0.9, h: 0.3, d: 0.25, wall: true, y: 2.0 },
      { key: 'hook', icon: '🪝', name: '挂钩', type: 'wall_mount', w: 0.4, h: 0.06, d: 0.03, wall: true, y: 1.7 },
    ],
  },
]

export const ALL_PRESETS = PRESET_GROUPS.flatMap(g => g.items)
