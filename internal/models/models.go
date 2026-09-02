// Package models 定义数据库表对应的结构体。
// 坐标语义：x/y/z 为物体中心点坐标，单位米；
// w 为 X 向宽度、d 为 Z 向深度、h 为 Y 向高度，rot_y 为绕 Y 轴旋转（弧度）。
package models

// Household 家：数据归属的顶层单位（产品说明书 §H 家庭共享模型）。
type Household struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// Member 家庭成员：预留，单用户阶段仅有 owner（user_id=1）。
type Member struct {
	ID          int64  `json:"id"`
	HouseholdID int64  `json:"household_id"`
	UserID      int64  `json:"user_id"`
	Role        string `json:"role"` // owner | admin | member
	Nickname    string `json:"nickname"`
}

// Room 房间：归属某个家；物理尺寸决定 3D 场景的墙体轮廓。
type Room struct {
	ID          int64   `json:"id"`
	HouseholdID int64   `json:"household_id"`
	Name        string  `json:"name"`
	RoomW       float64 `json:"room_w"`
	RoomD       float64 `json:"room_d"`
	RoomH       float64 `json:"room_h"`
}

// Location 容器/家具/障碍物/可用区域。
// 业务归属（parent_id）与几何坐标（x/y/z/...）两套逻辑完全解耦：
// 系统不校验坐标与 parent_id 一致性，由用户保证。
type Location struct {
	ID            int64   `json:"id"`
	HouseholdID   int64   `json:"household_id"`
	RoomID        int64   `json:"room_id"`
	ParentID      *int64  `json:"parent_id"` // nil 或 0 均视为根节点 depth=0
	Name          string  `json:"name"`
	LocationType  string  `json:"location_type"`
	X             float64 `json:"x"`
	Y             float64 `json:"y"`
	Z             float64 `json:"z"`
	W             float64 `json:"w"`
	H             float64 `json:"h"`
	D             float64 `json:"d"`
	RotY          float64 `json:"rot_y"`
	Color         string  `json:"color"`
	IsActivityZone int    `json:"is_activity_zone"`
	IsCardbox     int     `json:"is_cardbox"`
	// GeomGroup 几何联动组 ID：置物架(架体+层板)、床头柜(台面+抽屉)等组合
	// 的成员共享同一值；拖拽任一成员时组内整体平移。纯几何概念，与业务
	// 归属 parent_id 无关（furniture 按红线仍不能作为 parent_id）。
	GeomGroup string  `json:"geom_group,omitempty"`
}

// Item 物品：无 3D 几何尺寸渲染，仅归属到 location，不参与嵌套深度计算。
// 归属维度：household_id（家）、owner_id + is_private（公共/私有，产品说明书 §H）。
// 细节维度：item_w/d/h 三维尺寸（可选）、category 分类、tags 标签（JSON 数组文本）。
type Item struct {
	ID          int64   `json:"id"`
	HouseholdID int64   `json:"household_id"`
	LocationID  int64   `json:"location_id"`
	Name        string  `json:"name"`
	Remark      string  `json:"remark"`
	Qty         int     `json:"qty"`
	// ImageURL 物品图片的相对路径（如 /uploads/items/1_1725.jpg），空表示无图。
	ImageURL string `json:"image_url"`
	// StockGroup 同名库存组：在用装与补充装共享组名，构成"用完→取备用→补货"闭环。空表示不参与库存管理。
	StockGroup string `json:"stock_group"`
	// MinQty 组内总量的补货阈值：在用+补充总量 ≤ 该值时进待购清单。0 表示不提醒。
	MinQty int `json:"min_qty"`
	// OwnerID 私有物品的所有者 user_id；公共物品恒为 0。
	OwnerID int64 `json:"owner_id"`
	// IsPrivate 1=私有（仅 owner 可见），0=公共（全家共享）。
	IsPrivate int `json:"is_private"`
	// ItemW/ItemD/ItemH 物品三维尺寸（米），0 表示未填写（可选）。
	ItemW float64 `json:"item_w"`
	ItemD float64 `json:"item_d"`
	ItemH float64 `json:"item_h"`
	// Category 物品分类（衣物/书籍/电子/厨具/工具/饰品/药品/食材/其他）。
	Category string `json:"category"`
	// Tags 自定义标签，JSON 数组文本，如 ["换季","易碎"]。
	Tags string `json:"tags"`
}
