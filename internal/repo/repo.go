// Package repo 实现数据访问层：CRUD 与 service.Store 接口。
// 数据归属以「家」（household_id）为隔离边界；物品区分公共/私有（owner_id + is_private）。
package repo

import (
	"database/sql"
	"errors"
	"fmt"

	"zhengli/internal/db"
	"zhengli/internal/dialect"
	"zhengli/internal/models"
	"zhengli/internal/service"
)

// Repo 封装数据库访问。方言与归属边界在构造时固定。
type Repo struct {
	db *sql.DB
	// dia 数据库方言：SQLite/MySQL 差异统一经此处取 SQL 片段。
	dia *dialect.Dialect
	// householdID 当前家：单用户阶段固定为默认家，多成员扩展时改由会话决定。
	householdID int64
	// userID 当前用户：单用户阶段固定为 1（默认家 owner）。
	userID int64
}

// New 创建 Repo：解析默认家作为当前数据隔离边界。
func New(sqlDB *sql.DB, dia *dialect.Dialect) *Repo {
	hid, err := db.DefaultHouseholdID(sqlDB)
	if err != nil {
		// 理论不会发生：db.Open 已确保默认家存在。
		hid = 1
	}
	return &Repo{db: sqlDB, dia: dia, householdID: hid, userID: 1}
}

// visibleItemCond 物品可见条件：公共（is_private=0）或归属本人私有（owner_id=当前用户）。
// 单用户阶段当前用户即默认家 owner，条件恒成立；多成员时真正生效私有隔离。
func (r *Repo) visibleItemCond() string {
	return "(i.is_private = 0 OR i.owner_id = ?)"
}

var errNotFound = errors.New("记录不存在")

// ErrNotFound 对外暴露的未找到错误。
var ErrNotFound = errNotFound

// ---- service.Store 实现 ----

// GetLocation 实现 service.Store，供校验逻辑使用。
func (r *Repo) GetLocation(id int64) (*service.LocationRow, error) {
	row := r.db.QueryRow(
		`SELECT l.id, l.room_id, l.parent_id, l.name, l.location_type
		 FROM locations l WHERE l.id = ? AND l.deleted_at IS NULL`, id)
	var loc service.LocationRow
	err := row.Scan(&loc.ID, &loc.RoomID, &loc.ParentID, &loc.Name, &loc.LocationType)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("location %d: %w", id, errNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("查询 location %d: %w", id, err)
	}
	return &loc, nil
}

// CountTrayInRoom 实现 service.Store：统计房间内 tray 数量（排除 excludeID）。
func (r *Repo) CountTrayInRoom(roomID int64, excludeID int64) (int, error) {
	var n int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM locations
		 WHERE room_id = ? AND location_type = 'tray' AND id != ? AND deleted_at IS NULL`,
		roomID, excludeID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("统计 tray: %w", err)
	}
	return n, nil
}

// CountChildren 实现 service.Store：统计直属子容器数。
func (r *Repo) CountChildren(id int64) (int, error) {
	var n int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM locations WHERE parent_id = ? AND deleted_at IS NULL`, id).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("统计子容器: %w", err)
	}
	return n, nil
}

// CountItems 实现 service.Store：统计归属物品数。
func (r *Repo) CountItems(locationID int64) (int, error) {
	var n int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM items WHERE location_id = ? AND deleted_at IS NULL`, locationID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("统计物品: %w", err)
	}
	return n, nil
}

// ---- households ----

// ListHouseholds 返回全部家（单用户阶段仅默认家）。
func (r *Repo) ListHouseholds() ([]models.Household, error) {
	rows, err := r.db.Query(`SELECT id, name, COALESCE(created_at, '') FROM households ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("查询家列表: %w", err)
	}
	defer rows.Close()
	var out []models.Household
	for rows.Next() {
		var h models.Household
		if err := rows.Scan(&h.ID, &h.Name, &h.CreatedAt); err != nil {
			return nil, fmt.Errorf("扫描家行: %w", err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// CurrentHousehold 返回当前家信息。
func (r *Repo) CurrentHousehold() (*models.Household, error) {
	row := r.db.QueryRow(
		`SELECT id, name, COALESCE(created_at, '') FROM households WHERE id = ?`, r.householdID)
	var h models.Household
	if err := row.Scan(&h.ID, &h.Name, &h.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("household %d: %w", r.householdID, errNotFound)
		}
		return nil, fmt.Errorf("查询家 %d: %w", r.householdID, err)
	}
	return &h, nil
}

// ListMembers 返回当前家成员（单用户阶段仅 owner）。
func (r *Repo) ListMembers() ([]models.Member, error) {
	rows, err := r.db.Query(
		`SELECT id, household_id, user_id, role, COALESCE(nickname, '')
		 FROM members WHERE household_id = ? ORDER BY id`, r.householdID)
	if err != nil {
		return nil, fmt.Errorf("查询家庭成员: %w", err)
	}
	defer rows.Close()
	var out []models.Member
	for rows.Next() {
		var m models.Member
		if err := rows.Scan(&m.ID, &m.HouseholdID, &m.UserID, &m.Role, &m.Nickname); err != nil {
			return nil, fmt.Errorf("扫描成员行: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ---- rooms CRUD ----

// CreateRoom 新建房间（归属当前家）并返回带 ID 的对象。
func (r *Repo) CreateRoom(rm *models.Room) error {
	if rm.HouseholdID == 0 {
		rm.HouseholdID = r.householdID
	}
	res, err := r.db.Exec(
		`INSERT INTO rooms (household_id, name, room_w, room_d, room_h) VALUES (?, ?, ?, ?, ?)`,
		rm.HouseholdID, rm.Name, rm.RoomW, rm.RoomD, rm.RoomH)
	if err != nil {
		return fmt.Errorf("插入房间: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("获取房间 ID: %w", err)
	}
	rm.ID = id
	return nil
}

const roomCols = `id, COALESCE(household_id, 0), name, room_w, room_d, room_h`

func scanRoom(row interface{ Scan(...any) error }) (*models.Room, error) {
	var rm models.Room
	if err := row.Scan(&rm.ID, &rm.HouseholdID, &rm.Name, &rm.RoomW, &rm.RoomD, &rm.RoomH); err != nil {
		return nil, err
	}
	return &rm, nil
}

// ListRooms 返回当前家全部在册房间。
func (r *Repo) ListRooms() ([]models.Room, error) {
	rows, err := r.db.Query(
		`SELECT `+roomCols+` FROM rooms WHERE household_id = ? AND deleted_at IS NULL ORDER BY id`,
		r.householdID)
	if err != nil {
		return nil, fmt.Errorf("查询房间列表: %w", err)
	}
	defer rows.Close()
	var out []models.Room
	for rows.Next() {
		rm, err := scanRoom(rows)
		if err != nil {
			return nil, fmt.Errorf("扫描房间行: %w", err)
		}
		out = append(out, *rm)
	}
	return out, rows.Err()
}

// GetRoom 按 ID 查询在册房间。
func (r *Repo) GetRoom(id int64) (*models.Room, error) {
	row := r.db.QueryRow(
		`SELECT `+roomCols+` FROM rooms WHERE id = ? AND deleted_at IS NULL`, id)
	rm, err := scanRoom(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("room %d: %w", id, errNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("查询房间 %d: %w", id, err)
	}
	return rm, nil
}

// UpdateRoom 更新房间。
func (r *Repo) UpdateRoom(rm *models.Room) error {
	res, err := r.db.Exec(
		`UPDATE rooms SET name = ?, room_w = ?, room_d = ?, room_h = ? WHERE id = ?`,
		rm.Name, rm.RoomW, rm.RoomD, rm.RoomH, rm.ID)
	if err != nil {
		return fmt.Errorf("更新房间: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("room %d: %w", rm.ID, errNotFound)
	}
	return nil
}

// DeleteRoom 软删除房间（locations/items 保持原状，随房间一同隐藏；回收站可恢复或彻底删除）。
func (r *Repo) DeleteRoom(id int64) error {
	res, err := r.db.Exec(
		`UPDATE rooms SET deleted_at = `+r.dia.Now()+` WHERE id = ? AND deleted_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("删除房间: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("room %d: %w", id, errNotFound)
	}
	return nil
}

// ---- locations CRUD ----

// CreateLocation 新建容器，写入前由上层完成业务校验。
func (r *Repo) CreateLocation(l *models.Location) error {
	if l.HouseholdID == 0 {
		l.HouseholdID = r.householdID
	}
	res, err := r.db.Exec(
		`INSERT INTO locations
		 (household_id, room_id, parent_id, name, location_type, x, y, z, w, h, d, rot_y,
		  color, is_activity_zone, is_cardbox, geom_group)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		l.HouseholdID, l.RoomID, l.ParentID, l.Name, l.LocationType,
		l.X, l.Y, l.Z, l.W, l.H, l.D, l.RotY,
		l.Color, l.IsActivityZone, l.IsCardbox, l.GeomGroup)
	if err != nil {
		return fmt.Errorf("插入 location: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("获取 location ID: %w", err)
	}
	l.ID = id
	return nil
}

const locationCols = `l.id, COALESCE(l.household_id, 0), l.room_id, l.parent_id, l.name, l.location_type,
	l.x, l.y, l.z, l.w, l.h, l.d, l.rot_y, l.color, l.is_activity_zone, l.is_cardbox,
	COALESCE(l.geom_group, '')`

func scanLocation(row interface{ Scan(...any) error }) (*models.Location, error) {
	var l models.Location
	if err := row.Scan(&l.ID, &l.HouseholdID, &l.RoomID, &l.ParentID, &l.Name, &l.LocationType,
		&l.X, &l.Y, &l.Z, &l.W, &l.H, &l.D, &l.RotY,
		&l.Color, &l.IsActivityZone, &l.IsCardbox, &l.GeomGroup); err != nil {
		return nil, err
	}
	return &l, nil
}

// GetLocationFull 按 ID 查询在册 location。
func (r *Repo) GetLocationFull(id int64) (*models.Location, error) {
	row := r.db.QueryRow(
		`SELECT `+locationCols+` FROM locations l WHERE l.id = ? AND l.deleted_at IS NULL`, id)
	l, err := scanLocation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("location %d: %w", id, errNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("查询 location %d: %w", id, err)
	}
	return l, nil
}

// ListLocationsByRoom 返回房间内在册 location。
func (r *Repo) ListLocationsByRoom(roomID int64) ([]models.Location, error) {
	rows, err := r.db.Query(
		`SELECT `+locationCols+` FROM locations l WHERE l.room_id = ? AND l.deleted_at IS NULL ORDER BY l.id`, roomID)
	if err != nil {
		return nil, fmt.Errorf("查询房间 locations: %w", err)
	}
	defer rows.Close()
	var out []models.Location
	for rows.Next() {
		l, err := scanLocation(rows)
		if err != nil {
			return nil, fmt.Errorf("扫描 location 行: %w", err)
		}
		out = append(out, *l)
	}
	return out, rows.Err()
}

// UpdateLocation 更新容器，写入前由上层完成业务校验。
func (r *Repo) UpdateLocation(l *models.Location) error {
	res, err := r.db.Exec(
		`UPDATE locations SET
		 room_id = ?, parent_id = ?, name = ?, location_type = ?,
		 x = ?, y = ?, z = ?, w = ?, h = ?, d = ?, rot_y = ?,
		 color = ?, is_activity_zone = ?, is_cardbox = ?, geom_group = ?
		 WHERE id = ?`,
		l.RoomID, l.ParentID, l.Name, l.LocationType,
		l.X, l.Y, l.Z, l.W, l.H, l.D, l.RotY,
		l.Color, l.IsActivityZone, l.IsCardbox, l.GeomGroup, l.ID)
	if err != nil {
		return fmt.Errorf("更新 location: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("location %d: %w", l.ID, errNotFound)
	}
	return nil
}

// UpdateLocationGeom 仅更新几何坐标尺寸——拖拽保存高频调用，
// 按需求“布局调整与业务归属互相独立”，此路径跳过业务校验。
func (r *Repo) UpdateLocationGeom(id int64, x, y, z, w, h, d, rotY float64) error {
	_, err := r.db.Exec(
		`UPDATE locations SET x = ?, y = ?, z = ?, w = ?, h = ?, d = ?, rot_y = ?
		 WHERE id = ?`, x, y, z, w, h, d, rotY, id)
	if err != nil {
		return fmt.Errorf("更新几何坐标: %w", err)
	}
	return nil
}

// DeleteLocation 软删除容器及其整个下级子树（与文件系统目录删除语义一致）。
// 物品不软删（挂在原地，容器恢复即回来），但搜索会过滤已删容器内的物品。
func (r *Repo) DeleteLocation(id int64) error {
	ids, err := r.GetDescendantIDsAll(id)
	if err != nil {
		return err
	}
	ids = append(ids, id)
	q := `UPDATE locations SET deleted_at = ` + r.dia.Now() + `
	      WHERE deleted_at IS NULL AND id IN (` + placeholders(len(ids)) + `)`
	res, err := r.db.Exec(q, toAny(ids)...)
	if err != nil {
		return fmt.Errorf("删除 location: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("location %d: %w", id, errNotFound)
	}
	return nil
}

// GetDescendantIDsAll 递归收集全部下级 ID（不限软删状态，供级联软删/物理删除）。
func (r *Repo) GetDescendantIDsAll(id int64) ([]int64, error) {
	rows, err := r.db.Query(`
		WITH RECURSIVE sub(id) AS (
			SELECT l.id FROM locations l WHERE l.parent_id = ?
			UNION ALL
			SELECT l.id FROM locations l JOIN sub ON l.parent_id = sub.id
		) SELECT id FROM sub`, id)
	if err != nil {
		return nil, fmt.Errorf("递归收集下级: %w", err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var cid int64
		if err := rows.Scan(&cid); err != nil {
			return nil, err
		}
		out = append(out, cid)
	}
	return out, rows.Err()
}

func toAny(ids []int64) []any {
	out := make([]any, len(ids))
	for i, v := range ids {
		out[i] = v
	}
	return out
}

// ListChildren 查询在册直属子容器。
func (r *Repo) ListChildren(id int64) ([]models.Location, error) {
	rows, err := r.db.Query(
		`SELECT `+locationCols+` FROM locations l WHERE l.parent_id = ? AND l.deleted_at IS NULL ORDER BY l.id`, id)
	if err != nil {
		return nil, fmt.Errorf("查询子容器: %w", err)
	}
	defer rows.Close()
	var out []models.Location
	for rows.Next() {
		l, err := scanLocation(rows)
		if err != nil {
			return nil, fmt.Errorf("扫描子容器行: %w", err)
		}
		out = append(out, *l)
	}
	return out, rows.Err()
}

// ListLocationsByIDs 按一批 ID 查询 location（用于递归下级详情）。
func (r *Repo) ListLocationsByIDs(ids []int64) ([]models.Location, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	// ids 来自内部查询结果，拼接安全。
	q := `SELECT ` + locationCols + ` FROM locations l WHERE l.deleted_at IS NULL AND l.id IN (`
	args := make([]any, 0, len(ids))
	for i, id := range ids {
		if i > 0 {
			q += ","
		}
		q += "?"
		args = append(args, id)
	}
	q += `) ORDER BY l.id`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("批量查询 locations: %w", err)
	}
	defer rows.Close()
	var out []models.Location
	for rows.Next() {
		l, err := scanLocation(rows)
		if err != nil {
			return nil, fmt.Errorf("扫描 location 行: %w", err)
		}
		out = append(out, *l)
	}
	return out, rows.Err()
}

// DeleteLocationCascade 清空房间全部 locations（物理删除，连带 items 因外键级联删除）。
// 仅供 CSV“清空房间重导入”模式调用——该操作意图明确，不走回收站。
func (r *Repo) DeleteLocationCascade(roomID int64) error {
	if _, err := r.db.Exec(
		`DELETE FROM locations WHERE room_id = ? AND deleted_at IS NULL`, roomID); err != nil {
		return fmt.Errorf("清空房间 locations: %w", err)
	}
	return nil
}

// GetDescendantIDs 递归 CTE 查询全部在册下级容器 ID（不含自身）。
func (r *Repo) GetDescendantIDs(id int64) ([]int64, error) {
	rows, err := r.db.Query(
		`WITH RECURSIVE sub(id) AS (
			SELECT l.id FROM locations l WHERE l.parent_id = ? AND l.deleted_at IS NULL
			UNION ALL
			SELECT l.id FROM locations l JOIN sub ON l.parent_id = sub.id AND l.deleted_at IS NULL
		 )
		 SELECT id FROM sub`, id)
	if err != nil {
		return nil, fmt.Errorf("递归查询下级: %w", err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var cid int64
		if err := rows.Scan(&cid); err != nil {
			return nil, fmt.Errorf("扫描递归结果: %w", err)
		}
		out = append(out, cid)
	}
	return out, rows.Err()
}

// ---- items CRUD ----

// CreateItem 新建物品，写入前由上层完成归属校验。
// 公共/私有：is_private=1 且未指定 owner 时默认归属当前用户。
func (r *Repo) CreateItem(it *models.Item) error {
	if it.HouseholdID == 0 {
		it.HouseholdID = r.householdID
	}
	if it.IsPrivate == 1 && it.OwnerID == 0 {
		it.OwnerID = r.userID
	}
	res, err := r.db.Exec(
		`INSERT INTO items
		 (household_id, location_id, name, remark, qty, image_url, stock_group, min_qty,
		  owner_id, is_private, item_w, item_d, item_h, category, tags)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		it.HouseholdID, it.LocationID, it.Name, it.Remark, it.Qty, it.ImageURL, it.StockGroup, it.MinQty,
		it.OwnerID, it.IsPrivate, it.ItemW, it.ItemD, it.ItemH, it.Category, it.Tags)
	if err != nil {
		return fmt.Errorf("插入物品: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("获取物品 ID: %w", err)
	}
	it.ID = id
	return nil
}

// itemCols 全列查询；可能为 NULL 的列统一 COALESCE 兜底（老数据未填时为 0/''）。
const itemCols = `i.id, i.household_id, i.location_id, i.name, i.remark, i.qty,
	COALESCE(i.image_url, ''), COALESCE(i.stock_group, ''),
	COALESCE(i.min_qty, 0),
	COALESCE(i.owner_id, 0), COALESCE(i.is_private, 0),
	COALESCE(i.item_w, 0), COALESCE(i.item_d, 0), COALESCE(i.item_h, 0),
	COALESCE(i.category, ''), COALESCE(i.tags, '')`

func scanItem(row interface{ Scan(...any) error }) (*models.Item, error) {
	var it models.Item
	if err := row.Scan(&it.ID, &it.HouseholdID, &it.LocationID, &it.Name, &it.Remark, &it.Qty,
		&it.ImageURL, &it.StockGroup, &it.MinQty,
		&it.OwnerID, &it.IsPrivate, &it.ItemW, &it.ItemD, &it.ItemH, &it.Category, &it.Tags); err != nil {
		return nil, err
	}
	return &it, nil
}

// ListItemsByLocation 查询归属某容器的在册物品（公共/私有均可见）。
func (r *Repo) ListItemsByLocation(locationID int64) ([]models.Item, error) {
	rows, err := r.db.Query(
		`SELECT `+itemCols+` FROM items i WHERE i.location_id = ? AND i.deleted_at IS NULL ORDER BY i.id`,
		locationID)
	if err != nil {
		return nil, fmt.Errorf("查询物品: %w", err)
	}
	defer rows.Close()
	var out []models.Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, fmt.Errorf("扫描物品行: %w", err)
		}
		out = append(out, *it)
	}
	return out, rows.Err()
}

// ListItemsByLocations 查询一组容器的全部物品（含容器名，供搜索结果展示）。
func (r *Repo) ListItemsByLocations(locationIDs []int64) ([]models.Item, error) {
	if len(locationIDs) == 0 {
		return nil, nil
	}
	// locationIDs 全部来自内部递归查询结果，非用户输入，拼接 IN 列表安全。
	q := `SELECT ` + itemCols + ` FROM items i WHERE i.deleted_at IS NULL AND i.location_id IN (`
	args := make([]any, 0, len(locationIDs)+1)
	for i, id := range locationIDs {
		if i > 0 {
			q += ","
		}
		q += "?"
		args = append(args, id)
	}
	q += `) ORDER BY i.id`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("批量查询物品: %w", err)
	}
	defer rows.Close()
	var out []models.Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, fmt.Errorf("扫描物品行: %w", err)
		}
		out = append(out, *it)
	}
	return out, rows.Err()
}

// SearchItems 按名称模糊搜索当前家的在册物品（手机端主入口）。
// JOIN 过滤：所在容器或房间已进回收站的物品不出现在搜索结果；
// 私有过滤：他人私有物品不进入搜索结果（单用户阶段恒可见）。
func (r *Repo) SearchItems(keyword string) ([]models.Item, []models.Location, error) {
	rows, err := r.db.Query(
		`SELECT `+itemCols+` FROM items i
		 JOIN locations l ON l.id = i.location_id AND l.deleted_at IS NULL
		 JOIN rooms rm ON rm.id = l.room_id AND rm.deleted_at IS NULL
		 WHERE i.deleted_at IS NULL AND i.household_id = ?
		   AND `+r.visibleItemCond()+`
		   AND i.name LIKE `+r.dia.LikeExpr()+`
		 ORDER BY i.id`, r.householdID, r.userID, keyword)
	if err != nil {
		return nil, nil, fmt.Errorf("搜索物品: %w", err)
	}
	defer rows.Close()
	var items []models.Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, nil, fmt.Errorf("扫描搜索结果: %w", err)
		}
		items = append(items, *it)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	// 补齐各物品归属容器的信息，供前端展示面包屑。
	locMap := map[int64]bool{}
	for _, it := range items {
		locMap[it.LocationID] = true
	}
	var locs []models.Location
	for id := range locMap {
		l, err := r.GetLocationFull(id)
		if err != nil {
			return nil, nil, err
		}
		locs = append(locs, *l)
	}
	return items, locs, nil
}

// UpdateItem 更新物品。
func (r *Repo) UpdateItem(it *models.Item) error {
	if it.IsPrivate == 1 && it.OwnerID == 0 {
		it.OwnerID = r.userID
	}
	res, err := r.db.Exec(
		`UPDATE items SET
		 location_id = ?, name = ?, remark = ?, qty = ?, image_url = ?, stock_group = ?, min_qty = ?,
		 owner_id = ?, is_private = ?, item_w = ?, item_d = ?, item_h = ?, category = ?, tags = ?
		 WHERE id = ? AND deleted_at IS NULL`,
		it.LocationID, it.Name, it.Remark, it.Qty, it.ImageURL, it.StockGroup, it.MinQty,
		it.OwnerID, it.IsPrivate, it.ItemW, it.ItemD, it.ItemH, it.Category, it.Tags, it.ID)
	if err != nil {
		return fmt.Errorf("更新物品: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("item %d: %w", it.ID, errNotFound)
	}
	return nil
}

// SetItemImage 仅更新物品图片路径（上传/删除图片专用）。
func (r *Repo) SetItemImage(id int64, imageURL string) error {
	res, err := r.db.Exec(`UPDATE items SET image_url = ? WHERE id = ?`, imageURL, id)
	if err != nil {
		return fmt.Errorf("更新物品图片: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("item %d: %w", id, errNotFound)
	}
	return nil
}

// ---- 库存组（在用/补充装） ----

// StockCandidate 备用补充装候选（"用完了"时供挑选扣减来源）。
type StockCandidate struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Qty   int    `json:"qty"`
	Where string `json:"where"` // "容器 › 房间"
}

// ListStockCandidates 列出同一库存组内、其它位置且数量>0 的物品（当前家内）。
func (r *Repo) ListStockCandidates(itemID int64) ([]StockCandidate, error) {
	rows, err := r.db.Query(`
		SELECT i.id, i.name, i.qty, COALESCE(l.name,''), COALESCE(rm.name,'')
		FROM items i
		JOIN items me ON me.id = ?
		JOIN locations l ON l.id = i.location_id AND l.deleted_at IS NULL
		JOIN rooms rm ON rm.id = l.room_id AND rm.deleted_at IS NULL
		WHERE i.stock_group = me.stock_group AND me.stock_group IS NOT NULL AND me.stock_group != ''
		  AND i.id != me.id AND i.deleted_at IS NULL AND i.qty > 0
		  AND i.household_id = ?
		ORDER BY i.qty DESC`, itemID, r.householdID)
	if err != nil {
		return nil, fmt.Errorf("查询补充装候选: %w", err)
	}
	defer rows.Close()
	var out []StockCandidate
	for rows.Next() {
		var c StockCandidate
		var locName, roomName string
		if err := rows.Scan(&c.ID, &c.Name, &c.Qty, &locName, &roomName); err != nil {
			return nil, err
		}
		c.Where = locName + " · " + roomName
		out = append(out, c)
	}
	return out, rows.Err()
}

// ConsumeFrom "用完了→取备用"：从补充装 sourceID 扣 1 件，在用装 activeID 数量置 1。
func (r *Repo) ConsumeFrom(activeID, sourceID int64) error {
	// 校验同组。
	var grp sql.NullString
	err := r.db.QueryRow(`SELECT stock_group FROM items WHERE id = ? AND deleted_at IS NULL`, activeID).Scan(&grp)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("item %d: %w", activeID, errNotFound)
	}
	if err != nil {
		return err
	}
	if !grp.Valid || grp.String == "" {
		return errors.New("该物品未设置库存组")
	}
	var grp2 sql.NullString
	err = r.db.QueryRow(`SELECT stock_group FROM items WHERE id = ? AND deleted_at IS NULL`, sourceID).Scan(&grp2)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("item %d: %w", sourceID, errNotFound)
	}
	if err != nil {
		return err
	}
	if grp2.String != grp.String {
		return errors.New("补充装与在用装库存组不一致")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE items SET qty = qty - 1 WHERE id = ? AND qty > 0 AND deleted_at IS NULL`, sourceID)
	if err != nil {
		return fmt.Errorf("扣减补充装: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("补充装已无库存")
	}
	if _, err := tx.Exec(`UPDATE items SET qty = 1 WHERE id = ?`, activeID); err != nil {
		return fmt.Errorf("恢复在用装: %w", err)
	}
	return tx.Commit()
}

// Restock 补货：数量 +n（n≥1）。
func (r *Repo) Restock(itemID int64, n int) error {
	if n < 1 {
		n = 1
	}
	res, err := r.db.Exec(`UPDATE items SET qty = qty + ? WHERE id = ? AND deleted_at IS NULL`, n, itemID)
	if err != nil {
		return fmt.Errorf("补货: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("item %d: %w", itemID, errNotFound)
	}
	return nil
}

// MarkEmpty 用完且无备用：在用装清零，并确保 min_qty ≥ 1（进待购清单）。
func (r *Repo) MarkEmpty(itemID int64) error {
	res, err := r.db.Exec(
		`UPDATE items SET qty = 0, min_qty = CASE WHEN min_qty < 1 THEN 1 ELSE min_qty END
		 WHERE id = ? AND deleted_at IS NULL`, itemID)
	if err != nil {
		return fmt.Errorf("标记用完: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("item %d: %w", itemID, errNotFound)
	}
	return nil
}

// ShoppingListEntry 待购清单条目。
type ShoppingListEntry struct {
	StockGroup string `json:"stock_group"`
	Name       string `json:"name"`
	Total      int    `json:"total"`
	MinQty     int    `json:"min_qty"`
	Suggest    int    `json:"suggest"`
}

// ShoppingList 汇总当前家内所有"组内总量 ≤ 阈值"的库存组（跨房间）。
func (r *Repo) ShoppingList() ([]ShoppingListEntry, error) {
	rows, err := r.db.Query(`
		SELECT i.stock_group, MAX(i.name), SUM(i.qty), MAX(i.min_qty)
		FROM items i
		JOIN locations l ON l.id = i.location_id AND l.deleted_at IS NULL
		JOIN rooms rm ON rm.id = l.room_id AND rm.deleted_at IS NULL
		WHERE i.deleted_at IS NULL AND i.stock_group IS NOT NULL AND i.stock_group != ''
		  AND i.household_id = ?
		GROUP BY i.stock_group
		HAVING MAX(i.min_qty) > 0 AND SUM(i.qty) <= MAX(i.min_qty)`, r.householdID)
	if err != nil {
		return nil, fmt.Errorf("查询待购清单: %w", err)
	}
	defer rows.Close()
	var out []ShoppingListEntry
	for rows.Next() {
		var e ShoppingListEntry
		if err := rows.Scan(&e.StockGroup, &e.Name, &e.Total, &e.MinQty); err != nil {
			return nil, err
		}
		e.Suggest = e.MinQty*2 - e.Total
		if e.Suggest < 1 {
			e.Suggest = 1
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetItem 按 ID 查询单个在册物品。
func (r *Repo) GetItem(id int64) (*models.Item, error) {
	row := r.db.QueryRow(`SELECT `+itemCols+` FROM items i WHERE i.id = ? AND i.deleted_at IS NULL`, id)
	it, err := scanItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("item %d: %w", id, errNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("查询物品 %d: %w", id, err)
	}
	return it, nil
}

// DeleteItem 软删除物品（回收站可恢复/彻底删除）。
func (r *Repo) DeleteItem(id int64) error {
	res, err := r.db.Exec(
		`UPDATE items SET deleted_at = `+r.dia.Now()+` WHERE id = ? AND deleted_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("删除物品: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("item %d: %w", id, errNotFound)
	}
	return nil
}

// ---- 回收站 ----

// TrashEntry 回收站条目：id/名称/删除时间 + 归属信息（供恢复前辨认）。
type TrashEntry struct {
	Kind       string `json:"kind"` // room | location | item
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	DeletedAt  string `json:"deleted_at"`
	RoomID     int64  `json:"room_id,omitempty"`
	RoomName   string `json:"room_name,omitempty"`
	ParentName string `json:"parent_name,omitempty"`
	Type       string `json:"type,omitempty"`
	Qty        int    `json:"qty,omitempty"`
}

// ListTrash 汇总回收站：已软删的房间、容器、物品。
func (r *Repo) ListTrash() ([]TrashEntry, error) {
	var out []TrashEntry
	// 房间。
	rows, err := r.db.Query(
		`SELECT id, name, deleted_at FROM rooms WHERE household_id = ? AND deleted_at IS NOT NULL ORDER BY deleted_at DESC`,
		r.householdID)
	if err != nil {
		return nil, fmt.Errorf("查询回收站房间: %w", err)
	}
	for rows.Next() {
		var e TrashEntry
		if err := rows.Scan(&e.ID, &e.Name, &e.DeletedAt); err != nil {
			rows.Close()
			return nil, err
		}
		e.Kind = "room"
		out = append(out, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// 容器（带房间名与父容器名）。
	rows, err = r.db.Query(`
		SELECT l.id, l.name, l.deleted_at, l.room_id, l.location_type,
		       COALESCE(r.name, ''), COALESCE(p.name, '')
		FROM locations l
		LEFT JOIN rooms r ON r.id = l.room_id
		LEFT JOIN locations p ON p.id = l.parent_id
		WHERE l.household_id = ? AND l.deleted_at IS NOT NULL
		ORDER BY l.deleted_at DESC`, r.householdID)
	if err != nil {
		return nil, fmt.Errorf("查询回收站容器: %w", err)
	}
	for rows.Next() {
		var e TrashEntry
		if err := rows.Scan(&e.ID, &e.Name, &e.DeletedAt, &e.RoomID, &e.Type, &e.RoomName, &e.ParentName); err != nil {
			rows.Close()
			return nil, err
		}
		e.Kind = "location"
		out = append(out, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// 物品（带所在容器名）。
	rows, err = r.db.Query(`
		SELECT i.id, i.name, i.deleted_at, i.location_id, i.qty,
		       COALESCE(l.name, ''), COALESCE(r.name, '')
		FROM items i
		LEFT JOIN locations l ON l.id = i.location_id
		LEFT JOIN rooms r ON r.id = l.room_id
		WHERE i.household_id = ? AND i.deleted_at IS NOT NULL
		ORDER BY i.deleted_at DESC`, r.householdID)
	if err != nil {
		return nil, fmt.Errorf("查询回收站物品: %w", err)
	}
	for rows.Next() {
		var e TrashEntry
		var locID int64
		if err := rows.Scan(&e.ID, &e.Name, &e.DeletedAt, &locID, &e.Qty, &e.ParentName, &e.RoomName); err != nil {
			rows.Close()
			return nil, err
		}
		e.Kind = "item"
		out = append(out, e)
	}
	rows.Close()
	return out, rows.Err()
}

// Restore 恢复一条软删记录。item/location 恢复时若祖先仍在回收站，沿 parent 链一并恢复；
// 祖先已被物理删除（如清空重导入）则挂到房间顶层并保留原名。
func (r *Repo) Restore(kind string, id int64) error {
	switch kind {
	case "room":
		res, err := r.db.Exec(`UPDATE rooms SET deleted_at = NULL WHERE id = ? AND deleted_at IS NOT NULL`, id)
		if err != nil {
			return fmt.Errorf("恢复房间: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("room %d: %w", id, errNotFound)
		}
	case "location":
		// 先沿 parent 链恢复祖先（含房间）。
		if err := r.restoreAncestors(id); err != nil {
			return err
		}
		// 恢复自身 + 其下级子树（软删是级联的，恢复也级联）。
		ids, err := r.GetDescendantIDsAll(id)
		if err != nil {
			return err
		}
		ids = append(ids, id)
		q := `UPDATE locations SET deleted_at = NULL WHERE id IN (` + placeholders(len(ids)) + `)`
		res, err := r.db.Exec(q, toAny(ids)...)
		if err != nil {
			return fmt.Errorf("恢复容器: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("location %d: %w", id, errNotFound)
		}
	case "item":
		// 恢复其所在容器链（若容器在回收站）。
		var locID int64
		if err := r.db.QueryRow(`SELECT location_id FROM items WHERE id = ?`, id).Scan(&locID); err != nil {
			return fmt.Errorf("item %d: %w", id, errNotFound)
		}
		if err := r.restoreAncestors(locID); err != nil {
			return err
		}
		res, err := r.db.Exec(
			`UPDATE items SET deleted_at = NULL WHERE id = ? AND deleted_at IS NOT NULL`, id)
		if err != nil {
			return fmt.Errorf("恢复物品: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("item %d: %w", id, errNotFound)
		}
	default:
		return fmt.Errorf("未知类型: %s", kind)
	}
	return nil
}

// restoreAncestors 沿 parent 链向上恢复所有已软删的祖先容器（含所属房间）。
func (r *Repo) restoreAncestors(locationID int64) error {
	type row struct {
		id       int64
		parentID sql.NullInt64
		roomID   int64
		deleted  sql.NullString
	}
	cur := locationID
	for cur > 0 {
		var x row
		err := r.db.QueryRow(
			`SELECT id, parent_id, room_id, deleted_at FROM locations WHERE id = ?`, cur).Scan(
			&x.id, &x.parentID, &x.roomID, &x.deleted)
		if errors.Is(err, sql.ErrNoRows) {
			// 祖先被物理删除：把当前节点挂到房间顶层，停止上溯。
			if _, err := r.db.Exec(`UPDATE locations SET parent_id = NULL WHERE id = ?`, locationID); err != nil {
				return fmt.Errorf("祖先缺失改挂顶层: %w", err)
			}
			return nil
		}
		if err != nil {
			return err
		}
		if x.deleted.Valid {
			// 恢复该祖先。
			if _, err := r.db.Exec(`UPDATE locations SET deleted_at = NULL WHERE id = ?`, x.id); err != nil {
				return fmt.Errorf("恢复祖先 %d: %w", x.id, err)
			}
			// 其所属房间若也在回收站，一并恢复。
			if _, err := r.db.Exec(`UPDATE rooms SET deleted_at = NULL WHERE id = ?`, x.roomID); err != nil {
				return fmt.Errorf("恢复房间 %d: %w", x.roomID, err)
			}
		}
		if x.parentID.Valid {
			cur = x.parentID.Int64
		} else {
			break
		}
	}
	return nil
}

// Purge 彻底删除回收站一条记录（物理删除，级联清掉下级）。
func (r *Repo) Purge(kind string, id int64) error {
	switch kind {
	case "room":
		if _, err := r.db.Exec(`DELETE FROM rooms WHERE id = ? AND deleted_at IS NOT NULL`, id); err != nil {
			return fmt.Errorf("彻底删除房间: %w", err)
		}
	case "location":
		// 递归收集下级（含已软删的）后统一物理删除。
		ids, err := r.GetDescendantIDsAll(id)
		if err != nil {
			return err
		}
		ids = append(ids, id)
		q := `DELETE FROM locations WHERE id IN (` + placeholders(len(ids)) + `)`
		if _, err := r.db.Exec(q, toAny(ids)...); err != nil {
			return fmt.Errorf("彻底删除容器: %w", err)
		}
	case "item":
		if _, err := r.db.Exec(`DELETE FROM items WHERE id = ? AND deleted_at IS NOT NULL`, id); err != nil {
			return fmt.Errorf("彻底删除物品: %w", err)
		}
	default:
		return fmt.Errorf("未知类型: %s", kind)
	}
	return nil
}

// PurgeAll 清空回收站（全部物理删除）。
func (r *Repo) PurgeAll() error {
	// 先彻底删房间（级联 locations/items），再删孤儿 locations，再删 items。
	if _, err := r.db.Exec(`DELETE FROM rooms WHERE deleted_at IS NOT NULL`); err != nil {
		return fmt.Errorf("清空回收站房间: %w", err)
	}
	if _, err := r.db.Exec(`DELETE FROM locations WHERE deleted_at IS NOT NULL`); err != nil {
		return fmt.Errorf("清空回收站容器: %w", err)
	}
	if _, err := r.db.Exec(`DELETE FROM items WHERE deleted_at IS NOT NULL`); err != nil {
		return fmt.Errorf("清空回收站物品: %w", err)
	}
	return nil
}

func placeholders(n int) string {
	s := ""
	for i := 0; i < n; i++ {
		if i > 0 {
			s += ","
		}
		s += "?"
	}
	return s
}
