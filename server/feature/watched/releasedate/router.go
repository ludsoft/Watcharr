package releasedate

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sbondCo/Watcharr/domain"
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
	w := r.br.Router.Group("/watched").Use(authmiddleware.AuthRequired(r.br.Cfg))

	w.POST("finished_at_release", r.FinishContent)
	w.POST("season/finished_at_release", r.FinishSeason)
	w.POST("episode/finished_at_release", r.FinishEpisode)
}

// Respond with the watched entry, or the error (400 for request caused
// errors, 500 otherwise).
func respond(c *gin.Context, w domain.WatchedDto, err error) {
	if err != nil {
		var ue UserError
		if errors.As(err, &ue) {
			c.JSON(http.StatusBadRequest, router.ErrorResponse{Error: ue.Error()})
			return
		}
		slog.Error("finished at release: Failed", "error", err)
		c.JSON(http.StatusInternalServerError, router.ErrorResponse{Error: "failed to finish at release date"})
		return
	}
	c.JSON(http.StatusOK, w)
}

func (r *Router) FinishContent(c *gin.Context) {
	var req ContentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, router.ErrorResponse{Error: err.Error()})
		return
	}
	w, err := r.service.FinishContent(c.MustGet("userId").(uint), req)
	respond(c, w, err)
}

func (r *Router) FinishSeason(c *gin.Context) {
	var req SeasonRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, router.ErrorResponse{Error: err.Error()})
		return
	}
	w, err := r.service.FinishSeason(c.MustGet("userId").(uint), req)
	respond(c, w, err)
}

func (r *Router) FinishEpisode(c *gin.Context) {
	var req EpisodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, router.ErrorResponse{Error: err.Error()})
		return
	}
	w, err := r.service.FinishEpisode(c.MustGet("userId").(uint), req)
	respond(c, w, err)
}
