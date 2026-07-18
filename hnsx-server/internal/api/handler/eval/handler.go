package eval

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	edto "github.com/hnsx-io/hnsx/server/internal/api/dto/eval"
	"github.com/hnsx-io/hnsx/server/internal/domain/eval"
	esvc "github.com/hnsx-io/hnsx/server/internal/service/eval"
)

type Handler struct{ svc *esvc.Service }

func New(s *esvc.Service) *Handler { return &Handler{svc: s} }

func (h *Handler) Register(r gin.IRouter) {
	g := r.Group("/workspaces/:workspace_id/eval-sets")
	g.POST("", h.CreateSet)
	g.GET("", h.ListSets)
	g.GET(":id", h.GetSet)
	g.DELETE(":id", h.DeleteSet)
	g.GET(":id/runs", h.ListRuns)
	g.GET("/runs/:runId", h.GetRun)
}

func (h *Handler) CreateSet(c *gin.Context) {
	var req edto.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	in := esvc.CreateSetInput{
		WorkspaceID: c.Param("workspace_id"),
		Name:        req.Name,
		Description: req.Description,
		Version:     req.Version,
	}
	for _, k := range req.Cases {
		in.Cases = append(in.Cases, eval.Case{
			Name:     k.Name,
			Input:    k.Input,
			Expected: k.Expected,
			Scorer:   eval.ScorerKind(k.Scorer),
			Weight:   k.Weight,
		})
	}
	got, err := h.svc.CreateSet(c.Request.Context(), in)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, edto.FromDomainSet(got))
}

func (h *Handler) ListSets(c *gin.Context) {
	items, err := h.svc.ListSetsByWorkspace(c.Request.Context(), c.Param("workspace_id"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, edto.FromDomainSetList(items))
}

func (h *Handler) GetSet(c *gin.Context) {
	got, err := h.svc.GetSet(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, edto.FromDomainSet(got))
}

func (h *Handler) DeleteSet(c *gin.Context) {
	if err := h.svc.DeleteSet(c.Request.Context(), c.Param("id")); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) ListRuns(c *gin.Context) {
	items, err := h.svc.ListRuns(c.Request.Context(), c.Param("id"), 50)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, edto.FromDomainRunList(items))
}

func (h *Handler) GetRun(c *gin.Context) {
	got, err := h.svc.GetRun(c.Request.Context(), c.Param("runId"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, edto.FromDomainRun(got))
}

func writeError(c *gin.Context, err error) {
	if errors.Is(err, eval.ErrEvalSetNotFound) || errors.Is(err, eval.ErrEvalRunNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}