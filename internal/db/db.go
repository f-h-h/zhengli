// Package db 负责数据库连接管理与表结构初始化/迁移。
// 支持 SQLite（modernc.org/sqlite 纯 Go 驱动，默认）与 MySQL（go-sql-driver/mysql），
// 方言差异统一收敛到 internal/dialect 包，切换驱动只需改 ZHENGLI_DB_DRIVER 环境变量。
package db

import (
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"

	"zhengli/internal/dialect"
)

// column 迁移项：表/列/逻辑类型/默认值子句。
// 逻辑类型 REAL 会按方言映射（sqlite=REAL，mysql=DOUBLE）。
type column struct {
	table string
	col   string
	typ   string // "TEXT" | "INTEGER" | "REAL"
	def   string // 默认值子句，如 " NOT NULL DEFAULT 0"，可空
}

// buildSchema 按方言生成建表语句。
// 注意：不在此处建索引——索引引用 household_id 等后加列，
// 必须在增量迁移完成后创建，否则老库会报"no such column"。
func buildSchema(dia *dialect.Dialect) string {
	ai := dia.AutoIncrement()
	real := dia.RealType()
	return fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS households (
    id %s,
    name TEXT NOT NULL,
    created_at TEXT
);

CREATE TABLE IF NOT EXISTS members (
    id %s,
    household_id INTEGER NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL DEFAULT 1,
    role TEXT DEFAULT 'owner',
    nickname TEXT
);

CREATE TABLE IF NOT EXISTS rooms (
    id %s,
    household_id INTEGER,
    name TEXT NOT NULL,
    room_w %s,
    room_d %s,
    room_h %s
);

CREATE TABLE IF NOT EXISTS locations (
    id %s,
    household_id INTEGER,
    room_id INTEGER NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    parent_id INTEGER REFERENCES locations(id),
    name TEXT NOT NULL,
    location_type TEXT NOT NULL,
    x %s DEFAULT 0,
    y %s DEFAULT 0,
    z %s DEFAULT 0,
    w %s DEFAULT 0.2,
    h %s DEFAULT 0.2,
    d %s DEFAULT 0.2,
    rot_y %s DEFAULT 0,
    color TEXT DEFAULT "#cccccc",
    is_activity_zone INTEGER DEFAULT 0,
    is_cardbox INTEGER DEFAULT 0,
    geom_group TEXT
);

CREATE TABLE IF NOT EXISTS items (
    id %s,
    household_id INTEGER,
    location_id INTEGER NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    remark TEXT,
    qty INTEGER DEFAULT 1,
    image_url TEXT,
    stock_group TEXT,
    min_qty INTEGER NOT NULL DEFAULT 0,
    owner_id INTEGER,
    is_private INTEGER DEFAULT 0,
    item_w %s,
    item_d %s,
    item_h %s,
    category TEXT,
    tags TEXT
);
`, ai, ai, ai, real, real, real,
		ai, real, real, real, real, real, real, real,
		ai, real, real, real)
}

// buildIndexes 建索引语句：需在列迁移完成后执行。
func buildIndexes() string {
	return `
CREATE INDEX IF NOT EXISTS idx_rooms_household ON rooms(household_id);
CREATE INDEX IF NOT EXISTS idx_locations_room ON locations(room_id);
CREATE INDEX IF NOT EXISTS idx_locations_parent ON locations(parent_id);
CREATE INDEX IF NOT EXISTS idx_locations_household ON locations(household_id);
CREATE INDEX IF NOT EXISTS idx_items_location ON items(location_id);
CREATE INDEX IF NOT EXISTS idx_items_household ON items(household_id);
`
}

// Open 打开数据库并执行表结构初始化与增量迁移。
// driver: "sqlite"（默认）或 "mysql"；dsn 为驱动对应的连接串。
func Open(driver, dsn string) (*sql.DB, error) {
	if driver == "" {
		driver = "sqlite"
	}
	dia := dialect.For(driver)
	d, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库(%s): %w", driver, err)
	}
	// 外键约束：sqlite 需显式开启；mysql 默认开启。
	if s := dia.EnableFK(); s != "" {
		if _, err := d.Exec(s); err != nil {
			d.Close()
			return nil, fmt.Errorf("启用外键约束: %w", err)
		}
	}
	if _, err := d.Exec(buildSchema(dia)); err != nil {
		d.Close()
		return nil, fmt.Errorf("初始化表结构: %w", err)
	}
	// 增量迁移：老库逐列探测，缺失才补（不依赖驱动错误文本差异）。
	migrations := []column{
		{"items", "image_url", "TEXT", ""},
		{"locations", "geom_group", "TEXT", ""},
		{"rooms", "deleted_at", "TEXT", ""},
		{"locations", "deleted_at", "TEXT", ""},
		{"items", "deleted_at", "TEXT", ""},
		{"items", "stock_group", "TEXT", ""},
		{"items", "min_qty", "INTEGER", " NOT NULL DEFAULT 0"},
		// 家模型：全表归属默认家（老数据迁移见 ensureDefaultHousehold）。
		{"rooms", "household_id", "INTEGER", ""},
		{"locations", "household_id", "INTEGER", ""},
		{"items", "household_id", "INTEGER", ""},
		// 公共/私有物品。
		{"items", "owner_id", "INTEGER", ""},
		{"items", "is_private", "INTEGER", " DEFAULT 0"},
		// 物品细节扩展：三维尺寸/分类/标签。
		{"items", "item_w", "REAL", ""},
		{"items", "item_d", "REAL", ""},
		{"items", "item_h", "REAL", ""},
		{"items", "category", "TEXT", ""},
		{"items", "tags", "TEXT", ""},
	}
	for _, m := range migrations {
		exists, err := dia.HasColumn(d, m.table, m.col)
		if err != nil {
			d.Close()
			return nil, err
		}
		if exists {
			continue
		}
		typ := m.typ
		if typ == "REAL" {
			typ = dia.RealType()
		}
		ddl := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s%s", m.table, m.col, typ, m.def)
		if _, err := d.Exec(ddl); err != nil {
			d.Close()
			return nil, fmt.Errorf("迁移 %s.%s: %w", m.table, m.col, err)
		}
	}
	// 索引：必须在列迁移完成后创建（老库才有 household_id 等新列）。
	if _, err := d.Exec(buildIndexes()); err != nil {
		d.Close()
		return nil, fmt.Errorf("创建索引: %w", err)
	}
	// 家模型初始化：确保默认家存在，并把老数据归入默认家。
	if err := ensureDefaultHousehold(d); err != nil {
		d.Close()
		return nil, err
	}
	return d, nil
}

// DefaultHouseholdID 返回默认家 ID（无则先创建）。
func DefaultHouseholdID(d *sql.DB) (int64, error) {
	var id int64
	err := d.QueryRow(`SELECT id FROM households ORDER BY id LIMIT 1`).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("查询默认家: %w", err)
	}
	res, err := d.Exec(`INSERT INTO households (name, created_at) VALUES (?, strftime('%Y-%m-%dT%H:%M:%S','now'))`, "默认家庭")
	if err != nil {
		return 0, fmt.Errorf("创建默认家: %w", err)
	}
	id, err = res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取默认家 ID: %w", err)
	}
	return id, nil
}

// ensureDefaultHousehold 初始化默认家，并把尚未归属家（household_id IS NULL）的历史数据归入默认家。
func ensureDefaultHousehold(d *sql.DB) error {
	hid, err := DefaultHouseholdID(d)
	if err != nil {
		return err
	}
	// 默认家成员：确保 owner 用户存在。
	if _, err := d.Exec(
		`INSERT INTO members (household_id, user_id, role, nickname)
		 SELECT ?, 1, 'owner', '我' WHERE NOT EXISTS (
		   SELECT 1 FROM members WHERE household_id = ? AND user_id = 1)`,
		hid, hid); err != nil {
		return fmt.Errorf("初始化默认家成员: %w", err)
	}
	// 历史数据归入默认家。
	for _, table := range []string{"rooms", "locations", "items"} {
		if _, err := d.Exec(
			fmt.Sprintf(`UPDATE %s SET household_id = ? WHERE household_id IS NULL`, table), hid); err != nil {
			return fmt.Errorf("迁移 %s.household_id: %w", table, err)
		}
	}
	return nil
}
