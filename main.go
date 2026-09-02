// zhengli 家庭收纳位置管理系统入口：
// 启动 HTTP 服务，提供 REST API 并内嵌前端静态资源（web/dist）。
package main

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"zhengli/internal/db"
	"zhengli/internal/dialect"
	"zhengli/internal/handlers"
	"zhengli/internal/repo"
)

//go:embed all:web/dist
var webFS embed.FS

func main() {
	// 数据库驱动与连接：ZHENGLI_DB_DRIVER=sqlite|mysql（默认 sqlite）。
	// sqlite 走单文件 DSN；mysql 用 "user:pass@tcp(host:port)/db?parseTime=true"。
	driver := getenv("ZHENGLI_DB_DRIVER", "sqlite")
	dbPath := getenv("ZHENGLI_DB", "zhengli.db")
	dsn := dbPath
	if driver == "sqlite" && !strings.HasPrefix(dbPath, "file:") {
		dsn = "file:" + dbPath + "?_pragma=busy_timeout(5000)"
	}
	addr := getenv("ZHENGLI_ADDR", ":8080")

	d, err := db.Open(driver, dsn)
	if err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}
	defer d.Close()

	r := chi.NewRouter()
	h := handlers.New(repo.New(d, dialect.For(driver)))
	h.RegisterRoutes(r)

	// 前端静态资源：embed 到二进制，单文件部署。
	dist, err := fs.Sub(webFS, "web/dist")
	if err != nil {
		log.Fatalf("读取内嵌前端资源失败: %v", err)
	}
	fileServer := http.FileServer(http.FS(dist))
	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		// 优先返回真实文件，否则回退 index.html（前端路由）。
		// 注意去掉前导 "/"：fs.FS 路径不允许以 / 开头。
		p := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(req.URL.Path)), "/")
		if p == "" || p == "." {
			p = "index.html"
		}
		if _, err := fs.Stat(dist, p); err != nil {
			req.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, req)
	})

	// 上传图片的静态服务（本地磁盘目录，非内嵌）。
	r.Handle("/uploads/*", http.StripPrefix("/uploads/", http.FileServer(http.Dir("uploads"))))

	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("zhengli 服务已启动: http://localhost%s (db=%s)", addr, dbPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP 服务异常退出: %v", err)
		}
	}()

	// 优雅退出：Ctrl+C / 服务停止时先断流量再关库。
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("服务关闭异常: %v", err)
	}
}

// getenv 读取环境变量，缺省用 fallback。
func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
