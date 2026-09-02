// CSV 导入/导出 HTTP 端点：请求体为 CSV 原文本，响应为逐行结果。
package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"zhengli/internal/csvio"
	"zhengli/internal/models"
)

// importLocations POST /api/rooms/{roomID}/import/locations?mode=add|replace
// 请求体为 locations.csv 文本；mode=replace 清空房间后重导。
func (h *Handler) importLocations(w http.ResponseWriter, r *http.Request) {
	roomID, ok := urlID(r, "roomID")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的房间 ID"})
		return
	}
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "add"
	}
	if mode != "add" && mode != "replace" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "mode 仅支持 add/replace"})
		return
	}
	// 先确认房间存在，给出明确错误。
	if _, err := h.repo.GetRoom(roomID); err != nil {
		writeErr(w, err)
		return
	}
	res, err := csvio.ImportLocations(h.repo, r.Body, roomID, mode == "replace")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// importItems POST /api/import/items — 请求体为 items.csv 文本。
func (h *Handler) importItems(w http.ResponseWriter, r *http.Request) {
	res, err := csvio.ImportItems(h.repo, r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// exportLocations GET /api/rooms/{roomID}/export/locations — 下载 CSV。
func (h *Handler) exportLocations(w http.ResponseWriter, r *http.Request) {
	roomID, ok := urlID(r, "roomID")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的房间 ID"})
		return
	}
	locs, err := h.repo.ListLocationsByRoom(roomID)
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",
		`attachment; filename="locations_room_`+strconv.FormatInt(roomID, 10)+`.csv"`)
	_, _ = w.Write([]byte(csvio.ExportLocations(locs)))
}

// exportTreeItems GET /api/rooms/{roomID}/export/items — 全量物品清单（含归属链）。
func (h *Handler) exportTreeItems(w http.ResponseWriter, r *http.Request) {
	roomID, ok := urlID(r, "roomID")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的房间 ID"})
		return
	}
	locs, err := h.repo.ListLocationsByRoom(roomID)
	if err != nil {
		writeErr(w, err)
		return
	}
	nameOf := make(map[int64]string, len(locs))
	ids := make([]int64, 0, len(locs))
	for _, l := range locs {
		nameOf[l.ID] = l.Name
		ids = append(ids, l.ID)
	}
	items, err := h.repo.ListItemsByLocations(ids)
	if err != nil {
		writeErr(w, err)
		return
	}
	var b strings.Builder
	b.WriteString("id,name,qty,remark,location_id,location_name,item_w,item_d,item_h,category,tags,is_private,owner_id\n")
	// CSV 字段转义：名称/备注/分类/标签中可能含逗号、引号。
	esc := func(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
	// 尺寸为 0 视为未填写，导出留空。
	fnum := func(f float64) string {
		if f == 0 {
			return ""
		}
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	for _, it := range items {
		remark := esc(it.Remark)
		name := esc(it.Name)
		locName := esc(nameOf[it.LocationID])
		b.WriteString(strconv.FormatInt(it.ID, 10) + "," + name + "," +
			strconv.Itoa(it.Qty) + "," + remark + "," +
			strconv.FormatInt(it.LocationID, 10) + "," + locName + "," +
			fnum(it.ItemW) + "," + fnum(it.ItemD) + "," + fnum(it.ItemH) + "," +
			esc(it.Category) + "," + esc(it.Tags) + "," +
			strconv.Itoa(it.IsPrivate) + "," + strconv.FormatInt(it.OwnerID, 10) + "\n")
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",
		`attachment; filename="items_room_`+strconv.FormatInt(roomID, 10)+`.csv"`)
	_, _ = w.Write([]byte(b.String()))
}

// exportSnapshot GET /api/rooms/{roomID}/export/snapshot — 房间布局快照 JSON。
func (h *Handler) exportSnapshot(w http.ResponseWriter, r *http.Request) {
	roomID, ok := urlID(r, "roomID")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的房间 ID"})
		return
	}
	rm, err := h.repo.GetRoom(roomID)
	if err != nil {
		writeErr(w, err)
		return
	}
	locs, err := h.repo.ListLocationsByRoom(roomID)
	if err != nil {
		writeErr(w, err)
		return
	}
	ids := make([]int64, 0, len(locs))
	for _, l := range locs {
		ids = append(ids, l.ID)
	}
	items, err := h.repo.ListItemsByLocations(ids)
	if err != nil {
		writeErr(w, err)
		return
	}
	if locs == nil {
		locs = []models.Location{}
	}
	if items == nil {
		items = []models.Item{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"room":      rm,
		"locations": locs,
		"items":     items,
	})
}
