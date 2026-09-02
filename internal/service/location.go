// Package service 实现需求规格说明书中的核心业务校验：
// location_type 权限矩阵、嵌套深度硬限制（最多3层）、各类型特殊红线规则。
package service

import (
	"errors"
	"fmt"
)

// ErrUnsupportedChild 表示父容器类型不允许挂载子容器。
var ErrUnsupportedChild = errors.New("该类型不支持添加子容器")

// ErrDepthExceeded 表示业务嵌套深度超过系统上限。
var ErrDepthExceeded = errors.New("系统限制，嵌套层级最多3层")

// MaxNestingDepth 业务嵌套最大深度（根节点 depth=0，其直属子容器 depth=1）。
const MaxNestingDepth = 3

// typeRules location_type 权限矩阵，与需求 §2 一一对应。
// allowChild=允许挂载子容器；allowItem=允许存放物品。
type typeRule struct {
	AllowChild bool
	AllowItem  bool
}

var typeRules = map[string]typeRule{
	"cabinet":     {AllowChild: true, AllowItem: true},
	"drawer":      {AllowChild: true, AllowItem: true},
	"shelf_layer": {AllowChild: true, AllowItem: true},
	"box":         {AllowChild: true, AllowItem: true},
	"cardbox":     {AllowChild: true, AllowItem: true},
	"luggage":     {AllowChild: true, AllowItem: true},
	"wall_mount":  {AllowChild: true, AllowItem: true},
	"table":       {AllowChild: true, AllowItem: true},
	"hanger":      {AllowChild: false, AllowItem: true},
	"tray":        {AllowChild: false, AllowItem: false},
	"ground_spot": {AllowChild: false, AllowItem: true},
	"furniture":   {AllowChild: false, AllowItem: false},
	"obstacle":    {AllowChild: false, AllowItem: false},
	"door":        {AllowChild: false, AllowItem: false},
	"window":      {AllowChild: false, AllowItem: false},
}

// ValidTypes 返回全部合法 location_type 枚举值。
func ValidTypes() []string {
	return []string{
		"cabinet", "drawer", "shelf_layer", "box", "cardbox", "luggage",
		"wall_mount", "table", "hanger", "tray", "ground_spot",
		"furniture", "obstacle", "door", "window",
	}
}

// wallFeatureTypes 墙面特征类型：纯布局标记，必须顶层、禁存物、禁子容器。
var wallFeatureTypes = map[string]bool{"obstacle": true, "door": true, "window": true}

// IsValidType 校验 location_type 是否在枚举内。
func IsValidType(t string) bool {
	_, ok := typeRules[t]
	return ok
}

// AllowChild 返回该类型是否允许挂载子容器。
func AllowChild(t string) bool {
	r, ok := typeRules[t]
	return ok && r.AllowChild
}

// AllowItem 返回该类型是否允许存放物品。
func AllowItem(t string) bool {
	r, ok := typeRules[t]
	return ok && r.AllowItem
}

// locGetter 提供按 ID 查询 location 的最小接口，便于单测替换。
type locGetter interface {
	getLocation(id int64) (*LocationRow, error)
}

// LocationRow service 层内部使用的 location 行结构。
type LocationRow struct {
	ID           int64
	RoomID       int64
	ParentID     *int64
	Name         string
	LocationType string
}

// Store 抽象数据库访问，供校验逻辑与 handlers 共用。
type Store interface {
	GetLocation(id int64) (*LocationRow, error)
	// CountTrayInRoom 统计指定房间内 tray 记录数（排除指定 ID，用于更新场景）。
	CountTrayInRoom(roomID int64, excludeID int64) (int, error)
	// CountChildren 统计指定 location 的直属子容器数。
	CountChildren(id int64) (int, error)
	// CountItems 统计归属到指定 location 的物品数。
	CountItems(locationID int64) (int, error)
}

// getDepth 沿 parent_id 链向上递归求业务嵌套深度。
// 根节点（parent_id 为 nil/0）depth=0；规格 §3.1。
func getDepth(s Store, locID int64) (int, error) {
	depth := 0
	for locID != 0 {
		loc, err := s.GetLocation(locID)
		if err != nil {
			return 0, fmt.Errorf("getDepth 查询 %d: %w", locID, err)
		}
		if loc.ParentID == nil || *loc.ParentID == 0 {
			break
		}
		depth++
		locID = *loc.ParentID
	}
	return depth, nil
}

// ValidateLocation 创建/更新 location 前的完整校验，规格 §3.2。
// 更新场景（selfID>0）时 parent 不得指向自身或自己的下级，否则会形成环。
func ValidateLocation(s Store, roomID int64, selfID int64, parentID *int64, locType string) error {
	if !IsValidType(locType) {
		return fmt.Errorf("未知的 location_type: %s", locType)
	}

	// AI: 墙面特征红线——obstacle/door/window 的 parent_id 必须为 null，规格 §8-6。
	if wallFeatureTypes[locType] {
		if parentID != nil && *parentID != 0 {
			return errors.New(locType + " 的 parent_id 必须为 null")
		}
	}

	// AI: 更新时防环——parent 不能是自己，也不能是自己的下级。
	if selfID > 0 && parentID != nil && *parentID != 0 {
		if *parentID == selfID {
			return errors.New("parent_id 不能指向自身")
		}
		// 沿 parent 链上溯，若遇到 selfID 说明形成了环。
		cur := *parentID
		for cur != 0 {
			if cur == selfID {
				return errors.New("parent_id 不能指向自己的下级容器（会形成环）")
			}
			loc, err := s.GetLocation(cur)
			if err != nil {
				return fmt.Errorf("校验 parent 链查询 %d: %w", cur, err)
			}
			if loc.ParentID == nil || *loc.ParentID == 0 {
				break
			}
			cur = *loc.ParentID
		}
	}

	// 有父容器时：父类型权限 + 嵌套深度。
	if parentID != nil && *parentID != 0 {
		parent, err := s.GetLocation(*parentID)
		if err != nil {
			return fmt.Errorf("查询父容器 %d: %w", *parentID, err)
		}

		// AI: ground_spot 红线——箱子等容器不能把 parent_id 指向地面点位，规格 §8-4。
		if parent.LocationType == "ground_spot" {
			return errors.New("ground_spot 地面点位不允许作为父容器")
		}
		// AI: 门是障碍物级容器，唯一例外——门上可贴挂钩 wall_mount 挂东西。
		if parent.LocationType == "door" {
			if locType != "wall_mount" {
				return errors.New("门上只能贴挂钩（wall_mount）子容器")
			}
		} else if !AllowChild(parent.LocationType) {
			return ErrUnsupportedChild
		}
		// AI: 跨房间挂载无意义，父容器必须在同一房间。
		if parent.RoomID != roomID {
			return errors.New("父容器与当前容器不在同一房间")
		}

		parentDepth, err := getDepth(s, *parentID)
		if err != nil {
			return fmt.Errorf("计算父容器深度: %w", err)
		}
		if parentDepth+1 > MaxNestingDepth {
			return ErrDepthExceeded
		}

		// AI: 更新防绕过——父容器自身已是 depth=3 时，其下不允许再挂任何子容器。
		if parentDepth >= MaxNestingDepth {
			return ErrDepthExceeded
		}
	}

	// tray：同一房间最多一条；更新时排除自身。
	if locType == "tray" {
		n, err := s.CountTrayInRoom(roomID, selfID)
		if err != nil {
			return fmt.Errorf("统计房间 tray 数量: %w", err)
		}
		if n > 0 {
			return errors.New("同一房间最多只允许一个活动托盘 tray")
		}
	}

	return nil
}

// ValidateItemPlacement 校验物品能否归属到指定 location，规格红线 §8-2/3/6。
func ValidateItemPlacement(s Store, locationID int64) error {
	loc, err := s.GetLocation(locationID)
	if err != nil {
		return fmt.Errorf("查询归属容器 %d: %w", locationID, err)
	}
	if !AllowItem(loc.LocationType) {
		return fmt.Errorf("该类型（%s）禁止存放物品", loc.LocationType)
	}
	return nil
}

// ValidateDelete 校验删除 location 前的约束。
// 软删除模式下不再阻止有子容器/物品的删除——整个子树随之进回收站，可随时恢复；
// 误删防护由前端确认弹窗（展示将一并删除的子项数量）承担。
func ValidateDelete(s Store, id int64) error {
	_ = s
	_ = id
	return nil
}
