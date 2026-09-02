// 物品图片上传/删除端点：图片存本地 uploads/items/ 目录，DB 只存相对 URL。
package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// timeNowUnix 当前 Unix 时间戳（毫秒），用于生成不重复文件名。
func timeNowUnix() int64 { return time.Now().UnixMilli() }

// uploadDir 图片存储根目录（相对工作目录）。
const uploadDir = "uploads/items"

// maxImageSize 单图上限 10MB。
const maxImageSize = 10 << 20

// allowedExts 允许的图片扩展名。
var allowedExts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true}

// uploadItemImage POST /api/items/{itemID}/image (multipart 字段 file)
func (h *Handler) uploadItemImage(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r, "itemID")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的物品 ID"})
		return
	}

	if err := r.ParseMultipartForm(maxImageSize); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "解析上传失败: " + err.Error()})
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少 file 字段"})
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(hdr.Filename))
	if !allowedExts[ext] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "仅支持 jpg/png/webp/gif 图片"})
		return
	}
	if hdr.Size > maxImageSize {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "图片超过 10MB 上限"})
		return
	}

	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		writeErr(w, fmt.Errorf("创建上传目录: %w", err))
		return
	}

	// 文件名：物品ID_时间戳.扩展名，避免重名。
	name := fmt.Sprintf("%d_%d%s", id, timeNowUnix(), ext)
	path := filepath.Join(uploadDir, name)
	dst, err := os.Create(path)
	if err != nil {
		writeErr(w, fmt.Errorf("保存图片: %w", err))
		return
	}
	defer dst.Close()
	if _, err := io.Copy(dst, file); err != nil {
		writeErr(w, fmt.Errorf("写入图片: %w", err))
		return
	}

	// 更换图片时删除旧文件（失败不影响主流程，只留垃圾文件）。
	if old := h.currentImage(id); old != "" {
		removeUploadFile(old)
	}

	imageURL := "/" + filepath.ToSlash(path)
	if err := h.repo.SetItemImage(id, imageURL); err != nil {
		_ = os.Remove(path)
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"image_url": imageURL})
}

// deleteItemImage DELETE /api/items/{itemID}/image
func (h *Handler) deleteItemImage(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r, "itemID")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的物品 ID"})
		return
	}
	old := h.currentImage(id)
	if err := h.repo.SetItemImage(id, ""); err != nil {
		writeErr(w, err)
		return
	}
	removeUploadFile(old)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// currentImage 查询物品当前图片 URL，不存在/无图返回空串。
func (h *Handler) currentImage(itemID int64) string {
	it, err := h.repo.GetItem(itemID)
	if err != nil || it == nil {
		return ""
	}
	return it.ImageURL
}

// removeUploadFile 删除本地图片文件（仅允许 uploads/ 下的路径）。
func removeUploadFile(urlPath string) {
	if urlPath == "" || !strings.HasPrefix(urlPath, "/uploads/") {
		return
	}
	rel := strings.TrimPrefix(urlPath, "/")
	if strings.Contains(rel, "..") {
		return
	}
	_ = os.Remove(filepath.FromSlash(rel))
}
