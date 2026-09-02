// 回收站 API：软删记录的列表、恢复、彻底删除、清空。
package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"zhengli/internal/repo"
)

// listTrash GET /api/trash — 回收站全量列表。
func (h *Handler) listTrash(w http.ResponseWriter, r *http.Request) {
	entries, err := h.repo.ListTrash()
	if err != nil {
		writeErr(w, err)
		return
	}
	if entries == nil {
		entries = []repo.TrashEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
}

// restoreTrash POST /api/trash/{kind}/{id}/restore — 恢复一条记录（容器/物品恢复时联动祖先链）。
func (h *Handler) restoreTrash(w http.ResponseWriter, r *http.Request) {
	kind := chi.URLParam(r, "kind")
	id, ok := urlID(r, "id")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效 id"})
		return
	}
	if err := h.repo.Restore(kind, id); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// purgeTrash DELETE /api/trash/{kind}/{id} — 彻底删除一条（物理删除，级联下级）。
func (h *Handler) purgeTrash(w http.ResponseWriter, r *http.Request) {
	kind := chi.URLParam(r, "kind")
	id, ok := urlID(r, "id")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效 id"})
		return
	}
	if err := h.repo.Purge(kind, id); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// emptyTrash DELETE /api/trash — 清空回收站。
func (h *Handler) emptyTrash(w http.ResponseWriter, r *http.Request) {
	if err := h.repo.PurgeAll(); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
