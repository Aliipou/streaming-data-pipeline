package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/aliipou/streaming-data-pipeline/internal/models"
	"github.com/aliipou/streaming-data-pipeline/internal/processor"
	"github.com/aliipou/streaming-data-pipeline/internal/store"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Handler wires together the store, processor, and WebSocket hub.
type Handler struct {
	store     *store.Store
	processor *processor.Processor
	hub       *Hub
	log       *zap.Logger
}

// New creates a Handler.
func New(s *store.Store, p *processor.Processor, hub *Hub, log *zap.Logger) *Handler {
	return &Handler{store: s, processor: p, hub: hub, log: log}
}

// RegisterRoutes attaches all routes.
func (h *Handler) RegisterRoutes(r *gin.Engine) {
	v1 := r.Group("/api/v1")
	{
		v1.GET("/events", h.GetEvents)
		v1.GET("/anomalies", h.GetAnomalies)
		v1.GET("/stats", h.GetStats)
		v1.GET("/windows", h.GetWindows)
		v1.GET("/sensors", h.GetSensors)
	}
	r.GET("/ws", h.hub.ServeWS)
	r.Static("/web", "./web")
	r.GET("/", func(c *gin.Context) { c.Redirect(http.StatusFound, "/web/index.html") })
}

func (h *Handler) GetEvents(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	sType := c.Query("sensor_type")
	loc := c.Query("location")

	events, err := h.store.GetRecentEvents(c.Request.Context(), limit, sType, loc)
	if err != nil {
		h.log.Error("get events", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"events": events, "count": len(events)})
}

func (h *Handler) GetAnomalies(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	anomalies, err := h.store.GetRecentAnomalies(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"anomalies": anomalies, "count": len(anomalies)})
}

func (h *Handler) GetStats(c *gin.Context) {
	dbStats, err := h.store.GetStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	procStats := h.processor.GetStats()
	c.JSON(http.StatusOK, gin.H{
		"pipeline":       procStats,
		"database":       dbStats,
		"active_sensors": dbStats["active_sensors"],
	})
}

func (h *Handler) GetWindows(c *gin.Context) {
	windows := h.processor.GetWindows()
	c.JSON(http.StatusOK, gin.H{"windows": windows, "count": len(windows)})
}

func (h *Handler) GetSensors(c *gin.Context) {
	events, err := h.store.GetRecentEvents(c.Request.Context(), 1000, "", "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	seen := make(map[string]models.SensorEvent)
	for _, e := range events {
		if _, ok := seen[e.SensorID]; !ok {
			seen[e.SensorID] = e
		}
	}
	sensors := make([]models.SensorEvent, 0, len(seen))
	for _, e := range seen {
		sensors = append(sensors, e)
	}
	c.JSON(http.StatusOK, gin.H{"sensors": sensors, "count": len(sensors)})
}

// Logger middleware for Gin.
func Logger(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Info("request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
		)
	}
}

// CORS middleware.
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
