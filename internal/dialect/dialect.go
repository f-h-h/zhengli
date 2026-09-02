// Package dialect 抽象 SQLite/MySQL 方言差异，使数据访问层可切换数据库驱动。
// 仅封装与 SQL 语法强相关的差异点；业务查询仍以标准 SQL 编写。
package dialect

import (
	"database/sql"
	"fmt"
)

// Dialect 数据库方言：sqlite 或 mysql。
type Dialect struct {
	Name string // "sqlite" | "mysql"
}

// For 返回指定驱动的方言实现；未知驱动回退 SQLite（默认）。
func For(driver string) *Dialect {
	switch driver {
	case "mysql":
		return &Dialect{Name: "mysql"}
	default:
		return &Dialect{Name: "sqlite"}
	}
}

// IsMySQL 判断当前是否为 MySQL。
func (d *Dialect) IsMySQL() bool { return d.Name == "mysql" }

// Now 返回"当前时间戳"表达式，用于软删除时间戳写入。
// sqlite: strftime('%Y-%m-%dT%H:%M:%S','now')；mysql: CURRENT_TIMESTAMP。
func (d *Dialect) Now() string {
	if d.IsMySQL() {
		return "CURRENT_TIMESTAMP"
	}
	return "strftime('%Y-%m-%dT%H:%M:%S','now')"
}

// LikeExpr 返回 LIKE 右侧模糊匹配表达式（含一个 ? 占位符）。
// sqlite: '%' || ? || '%'；mysql: CONCAT('%', ?, '%')（避免 || 被当逻辑或）。
func (d *Dialect) LikeExpr() string {
	if d.IsMySQL() {
		return "CONCAT('%', ?, '%')"
	}
	return "'%' || ? || '%'"
}

// RealType 返回浮点列类型：sqlite=REAL，mysql=DOUBLE。
func (d *Dialect) RealType() string {
	if d.IsMySQL() {
		return "DOUBLE"
	}
	return "REAL"
}

// AutoIncrement 返回自增主键列定义。
func (d *Dialect) AutoIncrement() string {
	if d.IsMySQL() {
		return "BIGINT PRIMARY KEY AUTO_INCREMENT"
	}
	return "INTEGER PRIMARY KEY AUTOINCREMENT"
}

// EnableFK 返回连接初始化需执行的开外键语句；mysql 默认开启无需执行。
func (d *Dialect) EnableFK() string {
	if d.IsMySQL() {
		return ""
	}
	return "PRAGMA foreign_keys = ON"
}

// HasColumn 判断表是否存在指定列（增量迁移前探测）。
func (d *Dialect) HasColumn(db *sql.DB, table, column string) (bool, error) {
	if d.IsMySQL() {
		var n int
		err := db.QueryRow(
			`SELECT COUNT(*) FROM information_schema.columns
			 WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
			table, column).Scan(&n)
		if err != nil {
			return false, fmt.Errorf("检查 mysql 列 %s.%s: %w", table, column, err)
		}
		return n > 0, nil
	}
	// sqlite: PRAGMA table_info 返回 (cid, name, type, notnull, dflt_value, pk)。
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false, fmt.Errorf("检查 sqlite 列 %s.%s: %w", table, column, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return false, fmt.Errorf("扫描 sqlite 列信息: %w", err)
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}
