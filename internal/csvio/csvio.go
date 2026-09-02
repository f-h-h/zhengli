// Package csvio 实现 CSV 批量导入导出：rooms/locations/items 三套模板。
// locations 支持两套输入：直接中心点 x/y/z 优先；简易采集字段
// corner_x/corner_z/bottom_y/rot_y_deg 自动换算为中心点（规格 §6 模式B）。
package csvio

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"zhengli/internal/models"
	"zhengli/internal/service"
)

// RowError 单行导入错误：行号（1-based，含表头）+ 错误信息。
type RowError struct {
	Row int    `json:"row"`
	Msg  string `json:"msg"`
}

// ImportResult 导入结果汇总。
type ImportResult struct {
	Created int        `json:"created"`
	Updated int        `json:"updated"`
	Errors  []RowError `json:"errors"`
}

func addErr(res *ImportResult, row int, format string, args ...any) {
	res.Errors = append(res.Errors, RowError{Row: row, Msg: fmt.Sprintf(format, args...)})
}

// parseFloat 解析可空浮点字段，空串返回缺省 ok=false。
func parseFloat(s string) (float64, bool, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false, nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false, err
	}
	return v, true, nil
}

// parseInt 解析可空整数字段。
func parseInt(s string) (int64, bool, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false, nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false, err
	}
	return v, true, nil
}

// readAll 解析 CSV 并返回表头索引与数据行。
func readAll(input io.Reader) (map[string]int, [][]string, error) {
	r := csv.NewReader(input)
	// 统一列数由每行 len 判断；允许变长行。
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("解析 CSV: %w", err)
	}
	if len(records) < 1 {
		return nil, nil, fmt.Errorf("CSV 为空")
	}
	header := make(map[string]int, len(records[0]))
	for i, h := range records[0] {
		header[strings.TrimSpace(strings.ToLower(h))] = i
	}
	return header, records[1:], nil
}

func getCol(header map[string]int, rec []string, name string) string {
	if i, ok := header[name]; ok && i < len(rec) {
		return strings.TrimSpace(rec[i])
	}
	return ""
}

// hasCol 表头中是否存在该列。
func hasCol(header map[string]int, name string) bool {
	_, ok := header[name]
	return ok
}

// Importer 导入依赖的最小接口（复用 repo.Repo）。
type Importer interface {
	service.Store
	CreateLocation(l *models.Location) error
	UpdateLocation(l *models.Location) error
	GetLocationFull(id int64) (*models.Location, error)
	ListLocationsByRoom(roomID int64) ([]models.Location, error)
	DeleteLocationCascade(roomID int64) error
}

// ImportLocations 解析并导入 locations.csv。
// roomID：目标房间（CSV 中 room_id 列若存在则校验一致，规格模板不含 room_id 时以此为准）。
// replace=true 时先清空该房间全部 locations 再导入（清空房间重导入）。
func ImportLocations(imp Importer, input io.Reader, roomID int64, replace bool) (*ImportResult, error) {
	header, rows, err := readAll(input)
	if err != nil {
		return nil, err
	}

	if replace {
		if err := imp.DeleteLocationCascade(roomID); err != nil {
			return nil, fmt.Errorf("清空房间 locations: %w", err)
		}
	}

	res := &ImportResult{}
	for i, rec := range rows {
		rowNum := i + 2 // +2：跳过表头，1-based
		if len(strings.Join(rec, "")) == 0 {
			continue // 跳过空行
		}

		var l models.Location
		l.RoomID = roomID
		if hasCol(header, "room_id") {
			rid, ok, err := parseInt(getCol(header, rec, "room_id"))
			if err != nil {
				addErr(res, rowNum, "room_id 解析失败: %v", err)
				continue
			}
			if ok && rid != roomID {
				addErr(res, rowNum, "room_id(%d) 与目标房间(%d)不一致", rid, roomID)
				continue
			}
		}

		l.Name = getCol(header, rec, "name")
		if l.Name == "" {
			addErr(res, rowNum, "name 不能为空")
			continue
		}
		l.LocationType = getCol(header, rec, "location_type")
		if !service.IsValidType(l.LocationType) {
			addErr(res, rowNum, "未知的 location_type: %q", l.LocationType)
			continue
		}

		// 嵌套深度只需数字 ≤ MaxNestingDepth，此处直接解析 parent_id。
		if s := getCol(header, rec, "parent_id"); s != "" {
			pid, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				addErr(res, rowNum, "parent_id 解析失败: %v", err)
				continue
			}
			if pid != 0 {
				l.ParentID = &pid
			}
		}

		// 几何字段：默认值与表结构一致。
		l.W, l.H, l.D = 0.2, 0.2, 0.2
		l.Color = "#cccccc"

		var w, h, d float64
		var hasX, hasY, hasZ, hasW, hasH, hasD, hasRotY bool
		var x, y, z, rotY float64

		if x, hasX, err = parseFloat(getCol(header, rec, "x")); err != nil {
			addErr(res, rowNum, "x 解析失败: %v", err)
			continue
		}
		if y, hasY, err = parseFloat(getCol(header, rec, "y")); err != nil {
			addErr(res, rowNum, "y 解析失败: %v", err)
			continue
		}
		if z, hasZ, err = parseFloat(getCol(header, rec, "z")); err != nil {
			addErr(res, rowNum, "z 解析失败: %v", err)
			continue
		}
		if w, hasW, err = parseFloat(getCol(header, rec, "w")); err != nil {
			addErr(res, rowNum, "w 解析失败: %v", err)
			continue
		}
		if h, hasH, err = parseFloat(getCol(header, rec, "h")); err != nil {
			addErr(res, rowNum, "h 解析失败: %v", err)
			continue
		}
		if d, hasD, err = parseFloat(getCol(header, rec, "d")); err != nil {
			addErr(res, rowNum, "d 解析失败: %v", err)
			continue
		}
		if rotY, hasRotY, err = parseFloat(getCol(header, rec, "rot_y")); err != nil {
			addErr(res, rowNum, "rot_y 解析失败: %v", err)
			continue
		}

		// 简易采集字段（规格 §6 模式B）：corner_x/corner_z/bottom_y/rot_y_deg。
		var cornerX, cornerZ, bottomY, rotYDeg float64
		var hasCX, hasCZ, hasBY, hasRotDeg bool
		if cornerX, hasCX, err = parseFloat(getCol(header, rec, "corner_x")); err != nil {
			addErr(res, rowNum, "corner_x 解析失败: %v", err)
			continue
		}
		if cornerZ, hasCZ, err = parseFloat(getCol(header, rec, "corner_z")); err != nil {
			addErr(res, rowNum, "corner_z 解析失败: %v", err)
			continue
		}
		if bottomY, hasBY, err = parseFloat(getCol(header, rec, "bottom_y")); err != nil {
			addErr(res, rowNum, "bottom_y 解析失败: %v", err)
			continue
		}
		if rotYDeg, hasRotDeg, err = parseFloat(getCol(header, rec, "rot_y_deg")); err != nil {
			addErr(res, rowNum, "rot_y_deg 解析失败: %v", err)
			continue
		}

		if hasW {
			l.W = w
		}
		if hasH {
			l.H = h
		}
		if hasD {
			l.D = d
		}

		// AI: 换算优先级——直接中心点 x/y/z 优先于 corner 简易字段（规格 §6）。
		switch {
		case hasX && hasY && hasZ:
			l.X, l.Y, l.Z = x, y, z
		case hasCX && hasCZ && hasBY:
			l.X = cornerX + l.W/2
			l.Z = cornerZ + l.D/2
			l.Y = bottomY + l.H/2
		default:
			// 两者均未完整提供：保持默认 0（草图模式允许坐标缺失）。
		}
		if hasRotY {
			l.RotY = rotY
		} else if hasRotDeg {
			l.RotY = rotYDeg * math.Pi / 180
		}

		if s := getCol(header, rec, "color"); s != "" {
			l.Color = s
		}
		if isCardbox, ok, err := parseInt(getCol(header, rec, "is_cardbox")); err != nil {
			addErr(res, rowNum, "is_cardbox 解析失败: %v", err)
			continue
		} else if ok {
			l.IsCardbox = int(isCardbox)
		}
		// AI: cardbox 类型自动带上 is_cardbox 标记，简化用户录入。
		if l.LocationType == "cardbox" {
			l.IsCardbox = 1
		}
		if l.LocationType == "tray" {
			l.IsActivityZone = 1
		}

		// id 列存在且 >0 时按 ID 更新，否则新增。
		id, hasID, err := parseInt(getCol(header, rec, "id"))
		if err != nil {
			addErr(res, rowNum, "id 解析失败: %v", err)
			continue
		}

		if hasID && id > 0 {
			existing, err := imp.GetLocationFull(id)
			if err != nil {
				addErr(res, rowNum, "查询 id=%d 失败: %v", id, err)
				continue
			}
			if existing.RoomID != roomID {
				addErr(res, rowNum, "id=%d 不属于目标房间", id)
				continue
			}
			l.ID = id
			if err := service.ValidateLocation(imp, l.RoomID, id, l.ParentID, l.LocationType); err != nil {
				addErr(res, rowNum, "%s", err.Error())
				continue
			}
			if err := imp.UpdateLocation(&l); err != nil {
				addErr(res, rowNum, "更新失败: %v", err)
				continue
			}
			res.Updated++
		} else {
			if err := service.ValidateLocation(imp, l.RoomID, 0, l.ParentID, l.LocationType); err != nil {
				addErr(res, rowNum, "%s", err.Error())
				continue
			}
			if err := imp.CreateLocation(&l); err != nil {
				addErr(res, rowNum, "插入失败: %v", err)
				continue
			}
			res.Created++
		}
	}
	return res, nil
}

// ImportItems 解析并导入 items.csv。
// ItemImporter 提供 item 写入与归属校验所需查询。
type ItemImporter interface {
	service.Store
	CreateItem(it *models.Item) error
	UpdateItem(it *models.Item) error
}

// ImportItems 导入 items.csv；带 id 列时按 ID 更新。
func ImportItems(imp ItemImporter, input io.Reader) (*ImportResult, error) {
	header, rows, err := readAll(input)
	if err != nil {
		return nil, err
	}
	res := &ImportResult{}
	for i, rec := range rows {
		rowNum := i + 2
		if len(strings.Join(rec, "")) == 0 {
			continue
		}

		var it models.Item
		locID, _, err := parseInt(getCol(header, rec, "location_id"))
		if err != nil {
			addErr(res, rowNum, "location_id 解析失败: %v", err)
			continue
		}
		it.LocationID = locID
		it.Name = getCol(header, rec, "name")
		if it.Name == "" {
			addErr(res, rowNum, "name 不能为空")
			continue
		}
		if qty, ok, err := parseInt(getCol(header, rec, "qty")); err != nil {
			addErr(res, rowNum, "qty 解析失败: %v", err)
			continue
		} else if ok {
			it.Qty = int(qty)
		}
		if it.Qty < 1 {
			it.Qty = 1
		}
		it.Remark = getCol(header, rec, "remark")

		// 物品细节扩展：三维尺寸/分类/标签/公共私有。
		if v, ok, err := parseFloat(getCol(header, rec, "item_w")); err != nil {
			addErr(res, rowNum, "item_w 解析失败: %v", err)
			continue
		} else if ok {
			it.ItemW = v
		}
		if v, ok, err := parseFloat(getCol(header, rec, "item_d")); err != nil {
			addErr(res, rowNum, "item_d 解析失败: %v", err)
			continue
		} else if ok {
			it.ItemD = v
		}
		if v, ok, err := parseFloat(getCol(header, rec, "item_h")); err != nil {
			addErr(res, rowNum, "item_h 解析失败: %v", err)
			continue
		} else if ok {
			it.ItemH = v
		}
		it.Category = getCol(header, rec, "category")
		it.Tags = getCol(header, rec, "tags")
		if priv, ok, err := parseInt(getCol(header, rec, "is_private")); err != nil {
			addErr(res, rowNum, "is_private 解析失败: %v", err)
			continue
		} else if ok {
			it.IsPrivate = int(priv)
		}
		if oid, ok, err := parseInt(getCol(header, rec, "owner_id")); err != nil {
			addErr(res, rowNum, "owner_id 解析失败: %v", err)
			continue
		} else if ok {
			it.OwnerID = oid
		}

		// 归属校验：furniture/tray/obstacle 等禁止存放物品。
		if err := service.ValidateItemPlacement(imp, it.LocationID); err != nil {
			addErr(res, rowNum, "%s", err.Error())
			continue
		}

		if id, hasID, err := parseInt(getCol(header, rec, "id")); err != nil {
			addErr(res, rowNum, "id 解析失败: %v", err)
			continue
		} else if hasID && id > 0 {
			it.ID = id
			if err := imp.UpdateItem(&it); err != nil {
				addErr(res, rowNum, "更新失败: %v", err)
				continue
			}
			res.Updated++
		} else {
			if err := imp.CreateItem(&it); err != nil {
				addErr(res, rowNum, "插入失败: %v", err)
				continue
			}
			res.Created++
		}
	}
	return res, nil
}

// ExportLocations 全量导出 locations 为 CSV 文本（含 id 列，便于按 ID 回导更新）。
func ExportLocations(locs []models.Location) string {
	var b strings.Builder
	b.WriteString("id,room_id,parent_id,name,location_type,x,y,z,w,h,d,rot_y,color,is_activity_zone,is_cardbox\n")
	for _, l := range locs {
		pid := int64(0)
		if l.ParentID != nil {
			pid = *l.ParentID
		}
		fmt.Fprintf(&b, "%d,%d,%d,%q,%s,%s,%s,%s,%s,%s,%s,%s,%s,%d,%d\n",
			l.ID, l.RoomID, pid, l.Name, l.LocationType,
			strconv.FormatFloat(l.X, 'f', -1, 64),
			strconv.FormatFloat(l.Y, 'f', -1, 64),
			strconv.FormatFloat(l.Z, 'f', -1, 64),
			strconv.FormatFloat(l.W, 'f', -1, 64),
			strconv.FormatFloat(l.H, 'f', -1, 64),
			strconv.FormatFloat(l.D, 'f', -1, 64),
			strconv.FormatFloat(l.RotY, 'f', -1, 64),
			l.Color, l.IsActivityZone, l.IsCardbox)
	}
	return b.String()
}
