package analytics

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sbondCo/Watcharr/feature/auth/authmiddleware"
	"github.com/sbondCo/Watcharr/router"
)

type Router struct {
	br      *router.BaseRouter
	service *Service
}

func NewRouter(br *router.BaseRouter, service *Service) *Router {
	return &Router{br, service}
}

func (r *Router) AddRoutes() {
	analytics := r.br.Router.Group("/analytics").Use(authmiddleware.AuthRequired(r.br.Cfg))

	// Only ever returns the requesting users own analytics.
	analytics.GET("", r.GetAnalytics)
}

func (r *Router) GetAnalytics(c *gin.Context) {
	userId := c.MustGet("userId").(uint)
	var req Request
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, router.ErrorResponse{Error: err.Error()})
		return
	}
	resp, err := r.service.Get(userId, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, router.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}
