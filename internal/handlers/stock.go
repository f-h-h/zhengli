// 库存组 API：用完取备用（consume）、补货（restock）、标记需补货（empty）、待购清单。
package handlers

import (
	"net/http"

	"zhengli/internal/repo"
)

// consumeReq POST body：from_item_id 为空表示查询候选，非空表示执行扣减。
type consumeReq struct {
	FromItemID int64 `json:"from_item_id"`
}

// consumeItem POST /api/items/{id}/consume
// 无 body/无 from_item_id → 返回同组候选补充装；有 from_item_id → 扣减该补充装并恢复在用装。
func (h *Handler) consumeItem(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r, "itemID")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的物品 ID"})
		return
	}
	var body consumeReq
	_ = decodeBody(r, &body) // body 可为空（查询模式）

	if body.FromItemID == 0 {
		cands, err := h.repo.ListStockCandidates(id)
		if err != nil {
			writeErr(w, err)
			return
		}
		if cands == nil {
			cands = []repo.StockCandidate{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"candidates": cands})
		return
	}
	if err := h.repo.ConsumeFrom(id, body.FromItemID); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// restockItem POST /api/items/{id}/restock body {"qty": n} — 数量 +n（补充装入库）。
func (h *Handler) restockItem(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r, "itemID")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的物品 ID"})
		return
	}
	var body struct {
		Qty int `json:"qty"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if body.Qty < 1 {
		body.Qty = 1
	}
	if err := h.repo.Restock(id, body.Qty); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// markEmptyItem POST /api/items/{id}/empty — 用完且无备用：清零并确保进待购清单。
func (h *Handler) markEmptyItem(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r, "itemID")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的物品 ID"})
		return
	}
	if err := h.repo.MarkEmpty(id); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// shoppingList GET /api/shopping-list — 全部"总量 ≤ 阈值"的库存组。
func (h *Handler) shoppingList(w http.ResponseWriter, r *http.Request) {
	entries, err := h.repo.ShoppingList()
	if err != nil {
		writeErr(w, err)
		return
	}
	if entries == nil {
		entries = []repo.ShoppingListEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
}
