package service

import (
	"errors"
	"testing"
)

// mockStore 内存实现 service.Store，用于校验逻辑单测。
type mockStore struct {
	locations map[int64]*LocationRow
	children  map[int64]int
	items     map[int64]int
}

func newMockStore() *mockStore {
	return &mockStore{
		locations: map[int64]*LocationRow{},
		children:  map[int64]int{},
		items:     map[int64]int{},
	}
}

func (m *mockStore) addLoc(id int64, roomID int64, parentID *int64, name, typ string) {
	m.locations[id] = &LocationRow{ID: id, RoomID: roomID, ParentID: parentID, Name: name, LocationType: typ}
}

func (m *mockStore) getLocation(id int64) (*LocationRow, error) {
	l, ok := m.locations[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return l, nil
}

// GetLocation 实现 service.Store。
func (m *mockStore) GetLocation(id int64) (*LocationRow, error) { return m.getLocation(id) }

// CountTrayInRoom 实现 service.Store：基于内存 locations 真实过滤，
// 与 repo 的 `AND id != excludeID` 语义一致。
func (m *mockStore) CountTrayInRoom(roomID int64, excludeID int64) (int, error) {
	n := 0
	for _, l := range m.locations {
		if l.RoomID == roomID && l.LocationType == "tray" && l.ID != excludeID {
			n++
		}
	}
	return n, nil
}

// CountChildren 实现 service.Store。
func (m *mockStore) CountChildren(id int64) (int, error) { return m.children[id], nil }

// CountItems 实现 service.Store。
func (m *mockStore) CountItems(locationID int64) (int, error) { return m.items[locationID], nil }

func ptr(id int64) *int64 { return &id }

// TestValidateLocation 覆盖需求 §2/§3/§8 的红线规则。
func TestValidateLocation(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*mockStore)
		roomID  int64
		selfID  int64
		parent  *int64
		locType string
		wantErr string // 空串表示期望成功
	}{
		{
			name:    "根节点柜子创建成功",
			setup:   func(m *mockStore) {},
			roomID:  1,
			locType: "cabinet",
			wantErr: "",
		},
		{
			name: "父类型hanger禁止子容器",
			setup: func(m *mockStore) {
				m.addLoc(10, 1, nil, "落地衣架", "hanger")
			},
			roomID:  1,
			parent:  ptr(10),
			locType: "box",
			wantErr: "该类型不支持添加子容器",
		},
		{
			name: "父类型furniture禁止子容器",
			setup: func(m *mockStore) {
				m.addLoc(11, 1, nil, "床", "furniture")
			},
			roomID:  1,
			parent:  ptr(11),
			locType: "box",
			wantErr: "该类型不支持添加子容器",
		},
		{
			name: "父类型obstacle禁止子容器",
			setup: func(m *mockStore) {
				m.addLoc(12, 1, nil, "桌腿", "obstacle")
			},
			roomID:  1,
			parent:  ptr(12),
			locType: "box",
			wantErr: "该类型不支持添加子容器",
		},
		{
			name: "ground_spot禁止作为父容器",
			setup: func(m *mockStore) {
				m.addLoc(13, 1, nil, "床底空隙", "ground_spot")
			},
			roomID:  1,
			parent:  ptr(13),
			locType: "box",
			wantErr: "ground_spot 地面点位不允许作为父容器",
		},
		{
			name: "四层嵌套被拦截",
			setup: func(m *mockStore) {
				m.addLoc(20, 1, nil, "衣柜", "cabinet")                 // depth0
				m.addLoc(21, 1, ptr(20), "左上隔间", "drawer")          // depth1
				m.addLoc(22, 1, ptr(21), "收纳盒A", "box")             // depth2
				m.addLoc(23, 1, ptr(22), "分装小盒A1", "box")           // depth3
			},
			roomID:  1,
			parent:  ptr(23),
			locType: "box",
			wantErr: "系统限制，嵌套层级最多3层",
		},
		{
			name: "三层嵌套depth3的容器下不能再挂",
			setup: func(m *mockStore) {
				m.addLoc(30, 1, nil, "衣柜", "cabinet")
				m.addLoc(31, 1, ptr(30), "隔间", "drawer")
				m.addLoc(32, 1, ptr(31), "收纳盒", "box") // depth2
			},
			roomID:  1,
			parent:  ptr(32),
			locType: "box",
			wantErr: "",
		},
		{
			name: "obstacle的parent必须为null",
			setup: func(m *mockStore) {
				m.addLoc(40, 1, nil, "书桌", "table")
			},
			roomID:  1,
			parent:  ptr(40),
			locType: "obstacle",
			wantErr: "obstacle 的 parent_id 必须为 null",
		},
		{
			name: "door的parent必须为null",
			setup: func(m *mockStore) {
				m.addLoc(41, 1, nil, "书桌", "table")
			},
			roomID:  1,
			parent:  ptr(41),
			locType: "door",
			wantErr: "door 的 parent_id 必须为 null",
		},
		{
			name: "window的parent必须为null",
			setup: func(m *mockStore) {
				m.addLoc(42, 1, nil, "书桌", "table")
			},
			roomID:  1,
			parent:  ptr(42),
			locType: "window",
			wantErr: "window 的 parent_id 必须为 null",
		},
		{
			name:    "door顶层创建成功",
			setup:   func(m *mockStore) {},
			roomID:  1,
			locType: "door",
			wantErr: "",
		},
		{
			name: "门上贴挂钩wall_mount成功",
			setup: func(m *mockStore) {
				m.addLoc(43, 1, nil, "房门", "door")
			},
			roomID:  1,
			parent:  ptr(43),
			locType: "wall_mount",
			wantErr: "",
		},
		{
			name: "门上挂箱子被拦截",
			setup: func(m *mockStore) {
				m.addLoc(44, 1, nil, "房门", "door")
			},
			roomID:  1,
			parent:  ptr(44),
			locType: "box",
			wantErr: "门上只能贴挂钩（wall_mount）子容器",
		},
		{
			name:    "window顶层创建成功",
			setup:   func(m *mockStore) {},
			roomID:  1,
			locType: "window",
			wantErr: "",
		},
		{
			name:    "tray同房间已存在被拦截",
			setup:   func(m *mockStore) { m.addLoc(95, 1, nil, "已有托盘", "tray") },
			roomID:  1,
			locType: "tray",
			wantErr: "同一房间最多只允许一个活动托盘 tray",
		},
		{
			name:    "tray另一房间不受影响",
			setup:   func(m *mockStore) { m.addLoc(96, 1, nil, "另一房间托盘", "tray") },
			roomID:  2,
			locType: "tray",
			wantErr: "",
		},
		{
			name:    "tray更新时排除自身",
			setup:   func(m *mockStore) { m.addLoc(99, 1, nil, "本托盘", "tray") },
			roomID:  1,
			selfID:  99,
			locType: "tray",
			wantErr: "",
		},
		{
			name: "跨房间挂载被拦截",
			setup: func(m *mockStore) {
				m.addLoc(50, 2, nil, "另一房间的柜子", "cabinet")
			},
			roomID:  1,
			parent:  ptr(50),
			locType: "box",
			wantErr: "父容器与当前容器不在同一房间",
		},
		{
			name: "更新时parent指向自身被拦截",
			setup: func(m *mockStore) {
				m.addLoc(60, 1, nil, "衣柜", "cabinet")
			},
			roomID:  1,
			selfID:  60,
			parent:  ptr(60),
			locType: "cabinet",
			wantErr: "parent_id 不能指向自身",
		},
		{
			name: "更新时parent指向自己下级形成环被拦截",
			setup: func(m *mockStore) {
				m.addLoc(70, 1, nil, "衣柜", "cabinet")
				m.addLoc(71, 1, ptr(70), "隔间", "drawer")
			},
			roomID:  1,
			selfID:  70,
			parent:  ptr(71),
			locType: "cabinet",
			wantErr: "parent_id 不能指向自己的下级容器（会形成环）",
		},
		{
			name:    "未知类型被拦截",
			setup:   func(m *mockStore) {},
			roomID:  1,
			locType: "unknown_type",
			wantErr: "未知的 location_type: unknown_type",
		},
		{
			name: "合法三层链depth2的容器下挂载成功",
			setup: func(m *mockStore) {
				m.addLoc(80, 1, nil, "衣柜", "cabinet")
				m.addLoc(81, 1, ptr(80), "隔间", "drawer") // depth1
			},
			roomID:  1,
			parent:  ptr(81),
			locType: "box",
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newMockStore()
			tt.setup(m)
			err := ValidateLocation(m, tt.roomID, tt.selfID, tt.parent, tt.locType)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("期望成功，实际错误: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("期望错误 %q，实际成功", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Fatalf("错误信息不符：期望 %q，实际 %q", tt.wantErr, err.Error())
			}
		})
	}
}

// TestValidateItemPlacement 校验物品归属红线。
func TestValidateItemPlacement(t *testing.T) {
	tests := []struct {
		name    string
		locType string
		wantErr bool
	}{
		{"柜子可存物品", "cabinet", false},
		{"台面可存物品", "table", false},
		{"ground_spot可存物品", "ground_spot", false},
		{"hanger可存衣物", "hanger", false},
		{"furniture床禁止存物品", "furniture", true},
		{"tray禁止存物品", "tray", true},
		{"obstacle禁止存物品", "obstacle", true},
		{"door禁止存物品", "door", true},
		{"window禁止存物品", "window", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newMockStore()
			m.addLoc(1, 1, nil, "测试", tt.locType)
			err := ValidateItemPlacement(m, 1)
			if tt.wantErr && err == nil {
				t.Fatalf("期望错误，实际成功")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("期望成功，实际错误: %v", err)
			}
		})
	}
}

// TestGetDepth 校验递归深度计算（规格 §3.1）。
func TestGetDepth(t *testing.T) {
	m := newMockStore()
	m.addLoc(1, 1, nil, "衣柜", "cabinet")        // depth0
	m.addLoc(2, 1, ptr(1), "隔间", "drawer")       // depth1
	m.addLoc(3, 1, ptr(2), "收纳盒", "box")        // depth2
	m.addLoc(4, 1, ptr(3), "小盒", "box")          // depth3

	tests := []struct {
		locID int64
		want  int
	}{
		{1, 0}, {2, 1}, {3, 2}, {4, 3},
	}
	for _, tt := range tests {
		t.Run(string(rune('0'+tt.locID)), func(t *testing.T) {
			got, err := getDepth(m, tt.locID)
			if err != nil {
				t.Fatalf("getDepth(%d) 错误: %v", tt.locID, err)
			}
			if got != tt.want {
				t.Fatalf("getDepth(%d) = %d，期望 %d", tt.locID, got, tt.want)
			}
		})
	}
}

// TestValidateDelete 软删除模式下：删除一律放行（子树随容器进回收站，可恢复）。
func TestValidateDelete(t *testing.T) {
	t.Run("空容器可删除", func(t *testing.T) {
		m := newMockStore()
		m.addLoc(1, 1, nil, "柜子", "cabinet")
		if err := ValidateDelete(m, 1); err != nil {
			t.Fatalf("期望成功，实际错误: %v", err)
		}
	})
	t.Run("有子容器也放行（软删级联，可恢复）", func(t *testing.T) {
		m := newMockStore()
		m.addLoc(1, 1, nil, "柜子", "cabinet")
		m.children[1] = 2
		if err := ValidateDelete(m, 1); err != nil {
			t.Fatalf("期望成功，实际错误: %v", err)
		}
	})
	t.Run("有物品也放行（软删级联，可恢复）", func(t *testing.T) {
		m := newMockStore()
		m.addLoc(1, 1, nil, "柜子", "cabinet")
		m.items[1] = 5
		if err := ValidateDelete(m, 1); err != nil {
			t.Fatalf("期望成功，实际错误: %v", err)
		}
	})
}
