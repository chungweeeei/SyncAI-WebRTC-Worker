package api

import (
	"SyncAI-WebRTC-Worker/internal/api/handlers"
	"SyncAI-WebRTC-Worker/internal/config"
	"SyncAI-WebRTC-Worker/internal/gateway"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func NewRouter(app *config.AppConfig, gCfg *config.GStreamerConfig) *gin.Engine {

	router := gin.Default()
	router.Use(gin.Logger())
	router.Use(gin.Recovery())
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"https://*", "http://*", "http://localhost:5173"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowHeaders:     []string{"Content-Type", "Content-Length", "Authorization"},
		ExposeHeaders:    []string{"Location"},
		AllowCredentials: true,
	}))

	h := handlers.NewHandler(app, gCfg, &handlers.DefaultSessionManager{}, gateway.NewRobotGateway("/tmp/rtsp_pusher.sock"))

	router.POST("/control/whip", h.HandleControlWHIP)
	router.DELETE("/control/whip/:id", h.DeleteControlWHIP)

	router.POST("/audio/whip", h.HandleAudioWHIP)
	router.DELETE("/audio/whip/:id", h.DeleteAudioWHIP)

	router.POST("/video/whep", h.HandleVideoWHEP)
	router.DELETE("/video/whep/:id", h.DeleteVideoWHEP)

	return router
}
