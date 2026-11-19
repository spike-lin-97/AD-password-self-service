// Command password-self-service 密码自助服务平台（含飞书扫码登录 + AD 密码修改）。
package main

import (
	"context"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"password-self-service/api/v1/admin"
	"password-self-service/api/v1/auth"
	"password-self-service/internal/audit"
	"password-self-service/internal/config"
	"password-self-service/internal/session"
	"password-self-service/pkg/response"
	"password-self-service/web"

	"github.com/gin-gonic/gin"
)

var configFile = flag.String("config", "etc/password-self-service.yaml", "配置文件路径")

func main() {
	flag.Parse()

	cfg, err := config.Load(*configFile)
	if err != nil {
		log.Fatalf("load config failed: %v", err)
	}
	store := config.NewStore(*configFile, cfg)

	auditPath := filepath.Clean(filepath.Join(filepath.Dir(*configFile), "..", "var", "audit.jsonl"))
	auditStore, err := audit.New(auditPath)
	if err != nil {
		log.Fatalf("init audit log failed: %v", err)
	}
	log.Printf("audit log: %s", auditStore.Path())

	sessMgr, err := session.NewManager(cfg.Session.CookieName, cfg.Session.HashKey, cfg.Session.BlockKey, cfg.Session.MaxAge)
	if err != nil {
		log.Fatalf("init session failed: %v", err)
	}

	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	v1 := r.Group("/api/v1")
	{
		auth.NewHandler(store, sessMgr, auditStore).Register(v1)
		admin.NewHandler(store, sessMgr, auditStore).Register(v1)
	}

	r.GET("/", func(c *gin.Context) { serveWebFile(c, "index.html", "text/html; charset=utf-8") })
	r.GET("/index.html", func(c *gin.Context) { serveWebFile(c, "index.html", "text/html; charset=utf-8") })
	r.GET("/console.html", func(c *gin.Context) { serveWebFile(c, "console.html", "text/html; charset=utf-8") })
	r.GET("/admin.html", func(c *gin.Context) { serveWebFile(c, "admin.html", "text/html; charset=utf-8") })
	r.GET("/change-password.html", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/console.html")
	})
	r.GET("/favicon.ico", func(c *gin.Context) {
		data, err := web.StaticAsset.ReadFile("assets/img/favicon.svg")
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.Data(http.StatusOK, "image/svg+xml", data)
	})
	assetsFS, err := fs.Sub(web.StaticAsset, "assets")
	if err != nil {
		log.Fatalf("embed assets failed: %v", err)
	}
	r.StaticFS("/assets", http.FS(assetsFS))

	r.NoRoute(func(c *gin.Context) {
		response.Fail(c, http.StatusNotFound, "not_found", "接口不存在")
	})

	addr := cfg.Server.Addr
	if addr == "" {
		addr = ":8080"
	}
	srv := &http.Server{Addr: addr, Handler: r}

	go func() {
		log.Printf("Starting server on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("forced shutdown: %v", err)
	}
	log.Println("Server exited")
}

func serveWebFile(c *gin.Context, name, contentType string) {
	data, err := web.StaticAsset.ReadFile(name)
	if err != nil {
		response.Fail(c, http.StatusNotFound, "not_found", "页面不存在")
		return
	}
	c.Data(http.StatusOK, contentType, data)
}
