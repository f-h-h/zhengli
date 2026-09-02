// Package handlers 实现 REST API：rooms/locations/items 的 CRUD 与查询。
// 写操作统一走 service 校验层，保证需求红线在后端兜底拦截。
package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"zhengli/internal/models"
	"zhengli/internal/repo"
	"zhengli/internal/service"
)

// Handler HTTP 处理器，持有数据访问层。
type Handler struct {
	repo *repo.Repo
}

// New 创建 Handler。
func New(r *repo.Repo) *Handler {
	return &Handler{repo: r}
}

// RegisterRoutes 注册全部 REST 路由。
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/api", func(api chi.Router) {
		// 家（数据归属顶层单位，单用户阶段固定默认家）。
		api.Route("/households", func(hh chi.Router) {
			hh.Get("/", h.listHouseholds)
			hh.Get("/current", h.currentHousehold)
			hh.Get("/members", h.listMembers)
		})

		api.Route("/rooms", func(rooms chi.Router) {
			rooms.Get("/", h.listRooms)
			rooms.Post("/", h.createRoom)
			rooms.Route("/{roomID}", func(room chi.Router) {
				room.Get("/", h.getRoom)
				room.Put("/", h.updateRoom)
				room.Delete("/", h.deleteRoom)
				room.Get("/locations", h.listLocations)
				// CSV 导入导出。
				room.Post("/import/locations", h.importLocations)
				room.Get("/export/locations", h.exportLocations)
				room.Get("/export/items", h.exportTreeItems)
				room.Get("/export/snapshot", h.exportSnapshot)
			})
		})

		api.Route("/locations", func(locs chi.Router) {
			locs.Post("/", h.createLocation)
			locs.Route("/{locID}", func(loc chi.Router) {
				loc.Get("/", h.getLocation)
				loc.Put("/", h.updateLocation)
				loc.Patch("/geom", h.updateLocationGeom)
				loc.Delete("/", h.deleteLocation)
				// 查询：直属子容器 / 递归全部下级 / 面包屑 / 物品清单。
				loc.Get("/children", h.listChildren)
				loc.Get("/descendants", h.listDescendants)
				loc.Get("/breadcrumb", h.getBreadcrumb)
				loc.Get("/items", h.listItems)
				loc.Get("/tree-items", h.listTreeItems)
			})
		})

		api.Route("/items", func(items chi.Router) {
			items.Post("/", h.createItem)
			items.Post("/import", h.importItems)
			items.Get("/search", h.searchItems)
			items.Route("/{itemID}", func(item chi.Router) {
				item.Put("/", h.updateItem)
				item.Delete("/", h.deleteItem)
				// 图片上传/删除。
				item.Post("/image", h.uploadItemImage)
				item.Delete("/image", h.deleteItemImage)
				// 库存组操作。
				item.Post("/consume", h.consumeItem)
				item.Post("/restock", h.restockItem)
				item.Post("/empty", h.markEmptyItem)
			})
		})

		// 待购清单。
		api.Get("/shopping-list", h.shoppingList)

		// 回收站（软删除）。
		api.Route("/trash", func(trash chi.Router) {
			trash.Get("/", h.listTrash)
			trash.Delete("/", h.emptyTrash)
			trash.Post("/{kind}/{id}/restore", h.restoreTrash)
			trash.Delete("/{kind}/{id}", h.purgeTrash)
		})
	})
}

// ---- 通用辅助 ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeErr 统一错误响应：repo.ErrNotFound → 404，service 校验错误 → 400，其余 500。
func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, repo.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "记录不存在"})
	case errors.Is(err, service.ErrUnsupportedChild),
		errors.Is(err, service.ErrDepthExceeded),
		isValidationError(err):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
}

// isValidationError 判断是否为业务校验类错误（由 service 返回的普通错误）。
// service 校验错误均为 errors.New / fmt.Errorf 且非基础设施错误，
// 此处以“已知的哨兵错误或包含中文错误文案”简化处理：校验层错误统一走 400。
func isValidationError(err error) bool {
	// service 包的校验函数返回的错误不含 %w 包装的基础设施错误时按校验处理；
	// 基础设施错误（SQL 等）在 repo 层已用 %w 包装且不含中文提示，走 500。
	for _, target := range []error{service.ErrUnsupportedChild, service.ErrDepthExceeded} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

func urlID(r *http.Request, name string) (int64, bool) {
	v, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}

func decodeBody(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return errors.New("请求体 JSON 解析失败: " + err.Error())
	}
	return nil
}

// ---- households ----

func (h *Handler) listHouseholds(w http.ResponseWriter, r *http.Request) {
	hh, err := h.repo.ListHouseholds()
	if err != nil {
		writeErr(w, err)
		return
	}
	if hh == nil {
		hh = []models.Household{}
	}
	writeJSON(w, http.StatusOK, hh)
}

func (h *Handler) currentHousehold(w http.ResponseWriter, r *http.Request) {
	hhd, err := h.repo.CurrentHousehold()
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, hhd)
}

func (h *Handler) listMembers(w http.ResponseWriter, r *http.Request) {
	members, err := h.repo.ListMembers()
	if err != nil {
		writeErr(w, err)
		return
	}
	if members == nil {
		members = []models.Member{}
	}
	writeJSON(w, http.StatusOK, members)
}

// ---- rooms ----

func (h *Handler) listRooms(w http.ResponseWriter, r *http.Request) {
	rooms, err := h.repo.ListRooms()
	if err != nil {
		writeErr(w, err)
		return
	}
	if rooms == nil {
		rooms = []models.Room{}
	}
	writeJSON(w, http.StatusOK, rooms)
}

func (h *Handler) createRoom(w http.ResponseWriter, r *http.Request) {
	var rm models.Room
	if err := decodeBody(r, &rm); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if rm.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "房间名称不能为空"})
		return
	}
	if err := h.repo.CreateRoom(&rm); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, rm)
}

func (h *Handler) getRoom(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r, "roomID")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的房间 ID"})
		return
	}
	rm, err := h.repo.GetRoom(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rm)
}

func (h *Handler) updateRoom(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r, "roomID")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的房间 ID"})
		return
	}
	var rm models.Room
	if err := decodeBody(r, &rm); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if rm.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "房间名称不能为空"})
		return
	}
	rm.ID = id
	if err := h.repo.UpdateRoom(&rm); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rm)
}

func (h *Handler) deleteRoom(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r, "roomID")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的房间 ID"})
		return
	}
	if err := h.repo.DeleteRoom(id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- locations ----

func (h *Handler) listLocations(w http.ResponseWriter, r *http.Request) {
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
	if locs == nil {
		locs = []models.Location{}
	}
	writeJSON(w, http.StatusOK, locs)
}

func (h *Handler) createLocation(w http.ResponseWriter, r *http.Request) {
	var l models.Location
	if err := decodeBody(r, &l); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if l.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "容器名称不能为空"})
		return
	}
	if l.LocationType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "location_type 不能为空"})
		return
	}
	// 创建前完整业务校验：类型权限矩阵、嵌套深度、特殊红线。
	if err := service.ValidateLocation(h.repo, l.RoomID, 0, l.ParentID, l.LocationType); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if l.Color == "" {
		l.Color = "#cccccc"
	}
	if err := h.repo.CreateLocation(&l); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

func (h *Handler) getLocation(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r, "locID")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的容器 ID"})
		return
	}
	l, err := h.repo.GetLocationFull(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (h *Handler) updateLocation(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r, "locID")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的容器 ID"})
		return
	}
	var l models.Location
	if err := decodeBody(r, &l); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	l.ID = id
	if l.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "容器名称不能为空"})
		return
	}
	// 更新前同样执行完整校验（selfID=id 用于防环与 tray 排除自身）。
	if err := service.ValidateLocation(h.repo, l.RoomID, id, l.ParentID, l.LocationType); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := h.repo.UpdateLocation(&l); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, l)
}

// updateLocationGeom 拖拽保存专用：只改几何，跳过业务校验（需求：两套逻辑解耦）。
func (h *Handler) updateLocationGeom(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r, "locID")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的容器 ID"})
		return
	}
	var body struct {
		X, Y, Z, W, H, D, RotY float64
	}
	if err := decodeBody(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := h.repo.UpdateLocationGeom(id, body.X, body.Y, body.Z,
		body.W, body.H, body.D, body.RotY); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) deleteLocation(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r, "locID")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的容器 ID"})
		return
	}
	// 有子容器或有物品时拒绝删除，防止 ON DELETE CASCADE 误清数据。
	if err := service.ValidateDelete(h.repo, id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := h.repo.DeleteLocation(id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listChildren(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r, "locID")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的容器 ID"})
		return
	}
	// 直属子容器 = parent_id 指向本容器的全部 location。
	children, err := h.repo.ListChildren(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if children == nil {
		children = []models.Location{}
	}
	writeJSON(w, http.StatusOK, children)
}

func (h *Handler) listDescendants(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r, "locID")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的容器 ID"})
		return
	}
	ids, err := h.repo.GetDescendantIDs(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if len(ids) == 0 {
		writeJSON(w, http.StatusOK, []models.Location{})
		return
	}
	locs, err := h.repo.ListLocationsByIDs(ids)
	if err != nil {
		writeErr(w, err)
		return
	}
	if locs == nil {
		locs = []models.Location{}
	}
	writeJSON(w, http.StatusOK, locs)
}

// getBreadcrumb 返回业务归属 parent_id 链路（根 → 当前）。
func (h *Handler) getBreadcrumb(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r, "locID")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的容器 ID"})
		return
	}
	chain := []models.Location{}
	cur := id
	for cur != 0 {
		l, err := h.repo.GetLocationFull(cur)
		if err != nil {
			writeErr(w, err)
			return
		}
		chain = append(chain, *l)
		if l.ParentID == nil || *l.ParentID == 0 {
			break
		}
		cur = *l.ParentID
	}
	// 反转为根 → 当前。
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	writeJSON(w, http.StatusOK, chain)
}

func (h *Handler) listItems(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r, "locID")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的容器 ID"})
		return
	}
	items, err := h.repo.ListItemsByLocation(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if items == nil {
		items = []models.Item{}
	}
	writeJSON(w, http.StatusOK, items)
}

// listTreeItems 查询容器及其全部下级的物品清单。
func (h *Handler) listTreeItems(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r, "locID")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的容器 ID"})
		return
	}
	ids, err := h.repo.GetDescendantIDs(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	all := append([]int64{id}, ids...)
	items, err := h.repo.ListItemsByLocations(all)
	if err != nil {
		writeErr(w, err)
		return
	}
	if items == nil {
		items = []models.Item{}
	}
	writeJSON(w, http.StatusOK, items)
}

// ---- items ----

func (h *Handler) createItem(w http.ResponseWriter, r *http.Request) {
	var it models.Item
	if err := decodeBody(r, &it); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if it.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "物品名称不能为空"})
		return
	}
	if it.Qty < 1 {
		it.Qty = 1
	}
	// 归属校验：furniture/tray/obstacle 等禁止存放物品。
	if err := service.ValidateItemPlacement(h.repo, it.LocationID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := h.repo.CreateItem(&it); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, it)
}

func (h *Handler) updateItem(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r, "itemID")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的物品 ID"})
		return
	}
	var it models.Item
	if err := decodeBody(r, &it); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	it.ID = id
	if it.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "物品名称不能为空"})
		return
	}
	if it.Qty < 1 {
		it.Qty = 1
	}
	if err := service.ValidateItemPlacement(h.repo, it.LocationID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := h.repo.UpdateItem(&it); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, it)
}

func (h *Handler) deleteItem(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r, "itemID")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的物品 ID"})
		return
	}
	// 先取图片路径，删除记录后清理文件。
	old := h.currentImage(id)
	if err := h.repo.DeleteItem(id); err != nil {
		writeErr(w, err)
		return
	}
	removeUploadFile(old)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) searchItems(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少搜索关键词 q"})
		return
	}
	items, locs, err := h.repo.SearchItems(q)
	if err != nil {
		writeErr(w, err)
		return
	}
	if items == nil {
		items = []models.Item{}
	}
	if locs == nil {
		locs = []models.Location{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "locations": locs})
}
