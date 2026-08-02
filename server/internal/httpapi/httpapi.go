// Package httpapi wires the cvx HTTP contract onto an echo.Echo: digitize,
// tailor, render, persist, and (best-effort) email a tailored resume PDF.
// Handlers are thin — all domain logic lives in ai, pdfgen, and store, which
// are already unit-tested; this package only translates HTTP <-> those calls.
package httpapi

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"cvx/internal/ai"
	"cvx/internal/auth"
	"cvx/internal/jdfetch"
	"cvx/internal/model"
	"cvx/internal/pdfgen"
	"cvx/internal/store"
)

// maxUploadBytes caps profile PDF uploads; larger files get 413.
const maxUploadBytes = 15 << 20 // 15MB

// Server holds the dependencies the HTTP handlers need. Mail is injectable
// (it wraps mail.Send in main.go) so tests can fake it and so a failed send
// never fails the /api/generate request. coverPDF/coverFilename are ""/nil
// when the generation has no cover letter. Auth is required (not optional):
// it guards every /api/* route and serves the /auth/* OAuth entrypoints;
// see cvx/internal/auth for dev-mode vs OAuth-mode behavior.
type Server struct {
	Store *store.Store
	LLM   ai.LLM
	Mail  func(t model.Tailored, pdf []byte, filename string, coverPDF []byte, coverFilename string) (bool, error)
	Auth  *auth.Auth
}

func (s *Server) Register(e *echo.Echo) {
	// /auth/* is registered directly on e, outside the /api group, so it
	// never runs through the group's own auth middleware below — these
	// routes are how a session gets established in the first place.
	e.GET("/auth/providers", s.Auth.ListProviders)
	e.GET("/auth/:provider/start", s.Auth.AuthStart)
	e.GET("/auth/:provider/callback", s.Auth.AuthCallback)

	// /api/logout is also registered outside the guarded group: a bad or
	// expired session cookie must still be clearable, not stuck behind the
	// 401 it would itself cause.
	e.POST("/api/logout", s.Auth.Logout)

	api := e.Group("/api")
	api.Use(s.Auth.Middleware)
	api.GET("/me", s.Auth.GetMe)
	api.GET("/profile", s.getProfile)
	api.POST("/profile", s.postProfile)
	api.POST("/profile/extend", s.postProfileExtend)
	api.POST("/generate", s.postGenerate)
	api.GET("/generations", s.listGenerations)
	api.GET("/gaps", s.getGaps)
	api.GET("/generations/:id/pdf", s.getGenerationPDF)
	api.GET("/generations/:id/cover", s.getGenerationCoverPDF)
}

type profileSummary struct {
	Name       string `json:"name"`
	ItemCount  int    `json:"itemCount"`
	SkillCount int    `json:"skillCount"`
}

func summarize(p model.Profile) profileSummary {
	return profileSummary{Name: p.Name, ItemCount: len(p.Items), SkillCount: len(p.Skills)}
}

func errJSON(c echo.Context, status int, msg string) error {
	return c.JSON(status, map[string]string{"error": msg})
}

func (s *Server) getProfile(c echo.Context) error {
	p, err := s.Store.LoadProfile()
	if err != nil {
		slog.Error("load profile failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	if p == nil {
		return errJSON(c, http.StatusNotFound, "no profile")
	}
	return c.JSON(http.StatusOK, summarize(*p))
}

func (s *Server) postProfile(c echo.Context) error {
	fh, err := c.FormFile("file")
	if err != nil {
		return errJSON(c, http.StatusBadRequest, "missing or invalid file")
	}
	if fh.Size > maxUploadBytes {
		return errJSON(c, http.StatusRequestEntityTooLarge, "file too large")
	}

	f, err := fh.Open()
	if err != nil {
		return errJSON(c, http.StatusBadRequest, "missing or invalid file")
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return errJSON(c, http.StatusBadRequest, "missing or invalid file")
	}
	if len(data) > maxUploadBytes {
		return errJSON(c, http.StatusRequestEntityTooLarge, "file too large")
	}

	p, err := ai.Digitize(c.Request().Context(), s.LLM, data)
	if err != nil {
		slog.Error("digitize failed", "err", err)
		return errJSON(c, http.StatusBadGateway, err.Error())
	}
	if err := s.Store.SaveProfile(p); err != nil {
		slog.Error("save profile failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, summarize(p))
}

type extendRequest struct {
	Note string `json:"note"`
}

// postProfileExtend converts a typed note into profile additions via the
// LLM, merges them into the stored profile under model.MergeAdditions' id
// guardrail, and persists the result. A merge failure (unknown item id) is
// reported the same way as an LLM failure (502 + "profile extend failed")
// since both represent the LLM producing something we can't safely apply.
func (s *Server) postProfileExtend(c echo.Context) error {
	var req extendRequest
	if err := c.Bind(&req); err != nil {
		return errJSON(c, http.StatusBadRequest, "invalid request body")
	}
	if strings.TrimSpace(req.Note) == "" {
		return errJSON(c, http.StatusBadRequest, "note is required")
	}

	p, err := s.Store.LoadProfile()
	if err != nil {
		slog.Error("load profile failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	if p == nil {
		return errJSON(c, http.StatusConflict, "no profile")
	}

	additions, err := ai.ExtendProfile(c.Request().Context(), s.LLM, *p, req.Note)
	if err != nil {
		slog.Error("profile extend failed", "err", err)
		return errJSON(c, http.StatusBadGateway, err.Error())
	}
	if err := model.MergeAdditions(p, additions); err != nil {
		slog.Error("profile extend failed", "err", err)
		return errJSON(c, http.StatusBadGateway, err.Error())
	}
	if err := s.Store.SaveProfile(*p); err != nil {
		slog.Error("save profile failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, summarize(*p))
}

type generateRequest struct {
	RoleInput   string `json:"roleInput"`
	CoverLetter bool   `json:"coverLetter"`
}

type generateResponse struct {
	ID            string      `json:"id"`
	Filename      string      `json:"filename"`
	Gaps          []model.Gap `json:"gaps"`
	WhatChanged   []string    `json:"whatChanged"`
	Emailed       bool        `json:"emailed"`
	CoverFilename string      `json:"coverFilename"`
	CoverLetter   bool        `json:"coverLetter"`
}

func (s *Server) postGenerate(c echo.Context) error {
	var req generateRequest
	if err := c.Bind(&req); err != nil {
		return errJSON(c, http.StatusBadRequest, "invalid request body")
	}
	if strings.TrimSpace(req.RoleInput) == "" {
		return errJSON(c, http.StatusBadRequest, "roleInput is required")
	}

	ctx := c.Request().Context()
	if jdfetch.IsURL(req.RoleInput) {
		text, err := jdfetch.FetchText(ctx, req.RoleInput)
		if err != nil {
			slog.Error("jd fetch failed", "err", err)
			return errJSON(c, http.StatusBadGateway, "fetch job posting: "+err.Error())
		}
		req.RoleInput = text
	}

	p, err := s.Store.LoadProfile()
	if err != nil {
		slog.Error("load profile failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	if p == nil {
		return errJSON(c, http.StatusConflict, "no profile")
	}

	tailored, err := ai.Tailor(ctx, s.LLM, *p, req.RoleInput)
	if err != nil {
		slog.Error("tailor failed", "err", err)
		return errJSON(c, http.StatusBadGateway, err.Error())
	}

	pdf, err := pdfgen.Render(*p, tailored)
	if err != nil {
		slog.Error("render failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}

	filename := model.Filename(p.Name, tailored.TargetRole)

	// Cover letter generation is best-effort: its failure never fails the
	// resume generation, it just leaves coverPDF/coverFilename empty so the
	// response reports coverLetter=false.
	var coverPDF []byte
	var coverFilename string
	if req.CoverLetter {
		if cl, err := ai.CoverLetter(ctx, s.LLM, *p, req.RoleInput); err != nil {
			slog.Error("cover letter failed", "err", err)
		} else if rendered, err := pdfgen.RenderCoverLetter(*p, tailored.TargetRole, cl); err != nil {
			slog.Error("cover letter failed", "err", err)
		} else {
			coverPDF = rendered
			coverFilename = model.CoverFilename(p.Name, tailored.TargetRole)
		}
	}

	meta, err := s.Store.SaveGeneration(tailored, pdf, filename, coverPDF, coverFilename)
	if err != nil {
		slog.Error("save generation failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}

	emailed := false
	if s.Mail != nil {
		ok, err := s.Mail(tailored, pdf, filename, coverPDF, coverFilename)
		if err != nil {
			slog.Warn("email send failed", "err", err)
		}
		emailed = ok
	}

	return c.JSON(http.StatusOK, generateResponse{
		ID:            meta.ID,
		Filename:      meta.Filename,
		Gaps:          model.NonNil(tailored.Gaps),
		WhatChanged:   model.NonNil(tailored.WhatChanged),
		Emailed:       emailed,
		CoverFilename: coverFilename,
		CoverLetter:   coverFilename != "",
	})
}

func (s *Server) listGenerations(c echo.Context) error {
	list, err := s.Store.ListGenerations()
	if err != nil {
		slog.Error("list generations failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	if list == nil {
		list = []store.GenerationMeta{}
	}
	return c.JSON(http.StatusOK, list)
}

type gapsResponse struct {
	Total  int              `json:"total"`
	Trends []store.GapTrend `json:"trends"`
}

func (s *Server) getGaps(c echo.Context) error {
	trends, total, err := s.Store.GapSummary()
	if err != nil {
		slog.Error("gap summary failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, gapsResponse{Total: total, Trends: model.NonNil(trends)})
}

func (s *Server) getGenerationPDF(c echo.Context) error {
	id := c.Param("id")
	pdf, filename, err := s.Store.GetGenerationPDF(id)
	if err != nil {
		slog.Error("get generation pdf failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	if pdf == nil {
		return errJSON(c, http.StatusNotFound, "unknown generation id")
	}
	c.Response().Header().Set(echo.HeaderContentDisposition, fmt.Sprintf(`attachment; filename="%s"`, filename))
	return c.Blob(http.StatusOK, "application/pdf", pdf)
}

func (s *Server) getGenerationCoverPDF(c echo.Context) error {
	id := c.Param("id")
	pdf, filename, err := s.Store.GetGenerationCoverPDF(id)
	if err != nil {
		slog.Error("get generation cover pdf failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	if pdf == nil {
		return errJSON(c, http.StatusNotFound, "no cover letter for this generation")
	}
	c.Response().Header().Set(echo.HeaderContentDisposition, fmt.Sprintf(`attachment; filename="%s"`, filename))
	return c.Blob(http.StatusOK, "application/pdf", pdf)
}
