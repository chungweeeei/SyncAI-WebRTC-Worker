package api

import (
	"SyncAI-WebRTC-Worker/internal/config"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func NewRouter(app *config.AppConfig) *gin.Engine {

	router := gin.Default()
	router.Use(gin.Logger())
	router.Use(gin.Recovery())
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"https://*", "http://*", "http://localhost:5173"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Length", "Content-Type", "Authorization"},
		AllowCredentials: true,
	}))

	router.POST("/control/whip", handleControlWHIP)
	router.DELETE("/control/whip/:id", deleteControlWHIP)

	router.POST("/audio/whip", handleAudioWHIP)
	router.DELETE("/audio/whip/:id", deleteAudioWHIP)

	router.POST("/video/whep", handleVideoWHEP)
	router.DELETE("/video/whep/:id", deleteVideoWHEP)

	return router
}
