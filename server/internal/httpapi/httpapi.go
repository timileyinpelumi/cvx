// Package httpapi wires the cvx HTTP contract onto an echo.Echo: digitize,
// tailor, render, persist, and (best-effort) email a tailored resume PDF.
// Handlers are thin — all domain logic lives in ai, pdfgen, and store, which
// are already unit-tested; this package only translates HTTP <-> those calls.
package httpapi

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"cvx/internal/ai"
	"cvx/internal/auth"
	"cvx/internal/jdfetch"
	"cvx/internal/model"
	"cvx/internal/pdfgen"
	"cvx/internal/quality"
	"cvx/internal/store"
)

// maxUploadBytes caps profile PDF uploads; larger files get 413.
const maxUploadBytes = 15 << 20 // 15MB

// Fixed copy for rejected input (422): the classifier's reason is logged,
// never shown.
const (
	msgNotJobInput   = "That doesn't look like a job ad or a role. Paste the posting text, a link to it, or a role like Backend Engineer."
	msgNotResume     = "That PDF doesn't read like a resume. Upload the one you want cvx to work from."
	msgNoteNotUseful = "That note doesn't tell cvx anything about your experience. Say what you did, where, and with what."
)

// Server holds the dependencies the HTTP handlers need. Mail is injectable
// (it wraps mail.Send in main.go) so tests can fake it and so a failed send
// never fails the /api/generate request. coverPDF/coverFilename are ""/nil
// when the generation has no cover letter. Auth is required (not optional):
// it guards every /api/* route and serves the /auth/* OAuth entrypoints;
// see cvx/internal/auth for dev-mode vs OAuth-mode behavior.
type Server struct {
	Store *store.Store
	LLM   ai.LLM
	Mail  func(to string, t model.Tailored, pdf []byte, filename string, coverPDF []byte, coverFilename string) (bool, error)
	// RecruiterMail sends the forwardable recruiter-facing email instead of
	// the private notification when a generation requests it.
	RecruiterMail func(to, subject string, paragraphs []string, closing string, name string, pdf []byte, filename string, coverPDF []byte, coverFilename string) (bool, error)
	Auth          *auth.Auth
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
	api.GET("/profile/history", s.getProfileHistory)
	api.POST("/profile/restore", s.postProfileRestore)
	api.GET("/profile/skills", s.getSkills)
	api.PUT("/profile/skills", s.putSkills)
	api.POST("/generate", s.postGenerate)
	api.GET("/generations", s.listGenerations)
	api.GET("/gaps", s.getGaps)
	api.GET("/settings", s.getSettings)
	api.PUT("/settings", s.putSettings)
	api.GET("/settings/preview", s.getSettingsPreview)
	api.DELETE("/generations/:id", s.deleteGeneration)
	api.GET("/generations/:id/tailored", s.getGenerationTailored)
	api.PUT("/generations/:id", s.putGeneration)
	api.PUT("/generations/:id/pin", s.putGenerationPin)
	api.PUT("/generations/:id/status", s.putGenerationStatus)
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
	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		return errJSON(c, http.StatusUnauthorized, "unauthorized")
	}
	p, err := s.Store.LoadProfile(userID)
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
	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		return errJSON(c, http.StatusUnauthorized, "unauthorized")
	}
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
		if errors.Is(err, ai.ErrNotResume) {
			slog.Info("profile upload rejected", "err", err)
			return errJSON(c, http.StatusUnprocessableEntity, msgNotResume)
		}
		slog.Error("digitize failed", "err", err)
		return errJSON(c, http.StatusBadGateway, err.Error())
	}
	if err := s.Store.SaveProfile(userID, p); err != nil {
		slog.Error("save profile failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, summarize(p))
}

func (s *Server) getProfileHistory(c echo.Context) error {
	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		return errJSON(c, http.StatusUnauthorized, "unauthorized")
	}
	count, lastSavedAt, err := s.Store.ProfileHistoryInfo(userID)
	if err != nil {
		slog.Error("profile history info failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"count": count, "lastSavedAt": lastSavedAt})
}

func (s *Server) postProfileRestore(c echo.Context) error {
	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		return errJSON(c, http.StatusUnauthorized, "unauthorized")
	}
	p, err := s.Store.RestoreProfile(userID)
	if err != nil {
		slog.Error("profile restore failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	if p == nil {
		return errJSON(c, http.StatusNotFound, "no previous version")
	}
	return c.JSON(http.StatusOK, summarize(*p))
}

func (s *Server) getSkills(c echo.Context) error {
	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		return errJSON(c, http.StatusUnauthorized, "unauthorized")
	}
	p, err := s.Store.LoadProfile(userID)
	if err != nil {
		slog.Error("load profile failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	if p == nil {
		return errJSON(c, http.StatusNotFound, "no profile")
	}
	return c.JSON(http.StatusOK, map[string][]string{"skills": model.NonNil(p.Skills)})
}

const (
	maxSkills   = 200
	maxSkillLen = 60
)

func notSkillMsg(skill string) string {
	return fmt.Sprintf("%q doesn't look like a skill. Name a technology, tool, or method.", skill)
}

// putSkills replaces the profile's top-level skill list wholesale — one
// endpoint covers add, remove, and reorder. Skills embedded in bullets are
// untouched.
func (s *Server) putSkills(c echo.Context) error {
	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		return errJSON(c, http.StatusUnauthorized, "unauthorized")
	}
	var req struct {
		Skills []string `json:"skills"`
	}
	if err := c.Bind(&req); err != nil {
		return errJSON(c, http.StatusBadRequest, "invalid request body")
	}

	seen := map[string]bool{}
	skills := make([]string, 0, len(req.Skills))
	for _, raw := range req.Skills {
		skill := strings.TrimSpace(raw)
		if skill == "" {
			continue
		}
		if len(skill) > maxSkillLen {
			return errJSON(c, http.StatusBadRequest, fmt.Sprintf("skill %q is too long", skill))
		}
		key := strings.ToLower(skill)
		if seen[key] {
			continue
		}
		seen[key] = true
		skills = append(skills, skill)
	}
	if len(skills) > maxSkills {
		return errJSON(c, http.StatusBadRequest, "too many skills")
	}

	p, err := s.Store.LoadProfile(userID)
	if err != nil {
		slog.Error("load profile failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	if p == nil {
		return errJSON(c, http.StatusConflict, "no profile")
	}

	// Only genuinely new entries are judged, so removals and reorders of
	// skills already on the profile never cost a check or an LLM call.
	existing := map[string]bool{}
	for _, skill := range p.Skills {
		existing[strings.ToLower(skill)] = true
	}
	var additions []string
	for _, skill := range skills {
		if !existing[strings.ToLower(skill)] {
			additions = append(additions, skill)
		}
	}
	for _, skill := range additions {
		if !quality.SkillShaped(skill) {
			return errJSON(c, http.StatusUnprocessableEntity, notSkillMsg(skill))
		}
	}
	if len(additions) > 0 {
		invalid, err := ai.ClassifySkills(c.Request().Context(), s.LLM, additions)
		if err != nil {
			slog.Error("classify skills failed", "err", err)
			return errJSON(c, http.StatusBadGateway, err.Error())
		}
		if len(invalid) > 0 {
			slog.Info("skill addition rejected", "skills", invalid)
			return errJSON(c, http.StatusUnprocessableEntity, notSkillMsg(invalid[0]))
		}
	}

	p.Skills = skills
	if err := s.Store.SaveProfile(userID, *p); err != nil {
		slog.Error("save profile failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string][]string{"skills": skills})
}

type extendRequest struct {
	Note    string `json:"note"`
	Context string `json:"context"`
}

// postProfileExtend converts a typed note into profile additions via the
// LLM, merges them into the stored profile under model.MergeAdditions' id
// guardrail, and persists the result. A merge failure (unknown item id) is
// reported the same way as an LLM failure (502 + "profile extend failed")
// since both represent the LLM producing something we can't safely apply.
func (s *Server) postProfileExtend(c echo.Context) error {
	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		return errJSON(c, http.StatusUnauthorized, "unauthorized")
	}
	var req extendRequest
	if err := c.Bind(&req); err != nil {
		return errJSON(c, http.StatusBadRequest, "invalid request body")
	}
	if strings.TrimSpace(req.Note) == "" {
		return errJSON(c, http.StatusBadRequest, "note is required")
	}
	if quality.Mash(req.Note) {
		return errJSON(c, http.StatusUnprocessableEntity, msgNoteNotUseful)
	}

	p, err := s.Store.LoadProfile(userID)
	if err != nil {
		slog.Error("load profile failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	if p == nil {
		return errJSON(c, http.StatusConflict, "no profile")
	}

	additions, err := ai.ExtendProfile(c.Request().Context(), s.LLM, *p, req.Note, req.Context)
	if err != nil {
		if errors.Is(err, ai.ErrNoteNotUseful) {
			slog.Info("profile extend rejected", "err", err)
			return errJSON(c, http.StatusUnprocessableEntity, msgNoteNotUseful)
		}
		slog.Error("profile extend failed", "err", err)
		return errJSON(c, http.StatusBadGateway, err.Error())
	}
	if err := model.MergeAdditions(p, additions); err != nil {
		slog.Error("profile extend failed", "err", err)
		return errJSON(c, http.StatusBadGateway, err.Error())
	}
	if err := s.Store.SaveProfile(userID, *p); err != nil {
		slog.Error("save profile failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, summarize(*p))
}

type generateRequest struct {
	RoleInput      string `json:"roleInput"`
	CoverLetter    bool   `json:"coverLetter"`
	RecruiterEmail bool   `json:"recruiterEmail"`
}

type generateResponse struct {
	ID             string      `json:"id"`
	Filename       string      `json:"filename"`
	Gaps           []model.Gap `json:"gaps"`
	WhatChanged    []string    `json:"whatChanged"`
	Emailed        bool        `json:"emailed"`
	CoverFilename  string      `json:"coverFilename"`
	CoverLetter    bool        `json:"coverLetter"`
	RecruiterEmail bool        `json:"recruiterEmail"`
}

func (s *Server) postGenerate(c echo.Context) error {
	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		return errJSON(c, http.StatusUnauthorized, "unauthorized")
	}
	var req generateRequest
	if err := c.Bind(&req); err != nil {
		return errJSON(c, http.StatusBadRequest, "invalid request body")
	}
	if strings.TrimSpace(req.RoleInput) == "" {
		return errJSON(c, http.StatusBadRequest, "roleInput is required")
	}

	// Fingerprint the input as pasted (link or text, before any fetching) so
	// the same job twice refreshes one generation instead of adding another.
	fingerprint := store.RoleFingerprint(req.RoleInput)

	ctx := c.Request().Context()
	if jdfetch.IsURL(req.RoleInput) {
		text, err := jdfetch.FetchText(ctx, req.RoleInput)
		if err != nil {
			slog.Error("jd fetch failed", "err", err)
			return errJSON(c, http.StatusBadGateway, "fetch job posting: "+err.Error())
		}
		req.RoleInput = text
	} else if quality.Mash(req.RoleInput) {
		return errJSON(c, http.StatusUnprocessableEntity, msgNotJobInput)
	}

	p, err := s.Store.LoadProfile(userID)
	if err != nil {
		slog.Error("load profile failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	if p == nil {
		return errJSON(c, http.StatusConflict, "no profile")
	}

	if err := ai.ClassifyJobInput(ctx, s.LLM, req.RoleInput); err != nil {
		if errors.Is(err, ai.ErrNotJobInput) {
			slog.Info("job input rejected", "err", err)
			return errJSON(c, http.StatusUnprocessableEntity, msgNotJobInput)
		}
		slog.Error("classify job input failed", "err", err)
		return errJSON(c, http.StatusBadGateway, err.Error())
	}

	tailored, err := ai.TailorWithOptions(ctx, s.LLM, *p, req.RoleInput, s.loadGenerationParams(userID).options())
	if err != nil {
		slog.Error("tailor failed", "err", err)
		return errJSON(c, http.StatusBadGateway, err.Error())
	}
	// Production clamps to the content standard; the eval harness deliberately
	// judges the raw Tailor output, so this lives here and not inside ai.Tailor.
	model.NormalizeTailored(&tailored)

	style := s.loadStyle(userID)
	pdf, err := pdfgen.Render(*p, tailored, style)
	if err != nil {
		slog.Error("render failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}

	fileID := fileNumericID()
	filename := model.Filename(p.Name, tailored.TargetRole, fileID)

	// Cover letter generation is best-effort: its failure never fails the
	// resume generation, it just leaves coverPDF/coverFilename empty so the
	// response reports coverLetter=false.
	var coverPDF []byte
	var coverFilename string
	if req.CoverLetter {
		if cl, err := ai.CoverLetter(ctx, s.LLM, *p, req.RoleInput); err != nil {
			slog.Error("cover letter failed", "err", err)
		} else if rendered, err := pdfgen.RenderCoverLetter(*p, tailored.TargetRole, cl, style); err != nil {
			slog.Error("cover letter failed", "err", err)
		} else {
			coverPDF = rendered
			coverFilename = model.CoverFilename(p.Name, tailored.TargetRole, fileID)
		}
	}

	meta, err := s.Store.SaveGeneration(userID, tailored, pdf, filename, coverPDF, coverFilename, fingerprint)
	if err != nil {
		slog.Error("save generation failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}

	emailed := false
	recruiterSent := false
	if s.Mail != nil || s.RecruiterMail != nil {
		u, err := s.Store.GetUser(userID)
		if err != nil {
			slog.Warn("email recipient lookup failed", "err", err)
		} else if u != nil && u.Email != "" {
			if req.RecruiterEmail {
				if s.RecruiterMail != nil {
					// The user asked for the forwardable artifact: a failed
					// generation sends nothing rather than falling back to
					// the private notification.
					re, err := ai.RecruiterEmail(ctx, s.LLM, *p, req.RoleInput)
					if err != nil {
						slog.Error("recruiter email failed", "err", err)
					} else {
						ok, err := s.RecruiterMail(u.Email, re.Subject, re.Paragraphs, re.Closing, p.Name, pdf, filename, coverPDF, coverFilename)
						if err != nil {
							slog.Warn("email send failed", "err", err)
						}
						emailed = ok
						recruiterSent = ok
					}
				}
			} else if s.Mail != nil {
				// The notification copy is opt-out via settings; the
				// recruiter email above is an explicit per-generation ask
				// and ignores this preference.
				emailCopy, err := s.Store.GetEmailCopy(userID)
				if err != nil {
					slog.Warn("email copy setting lookup failed", "err", err)
					emailCopy = true
				}
				if emailCopy {
					ok, err := s.Mail(u.Email, tailored, pdf, filename, coverPDF, coverFilename)
					if err != nil {
						slog.Warn("email send failed", "err", err)
					}
					emailed = ok
				}
			}
		}
	}

	return c.JSON(http.StatusOK, generateResponse{
		ID:             meta.ID,
		Filename:       meta.Filename,
		Gaps:           model.NonNil(tailored.Gaps),
		WhatChanged:    model.NonNil(tailored.WhatChanged),
		Emailed:        emailed,
		CoverFilename:  coverFilename,
		CoverLetter:    coverFilename != "",
		RecruiterEmail: recruiterSent,
	})
}

func (s *Server) getGenerationTailored(c echo.Context) error {
	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		return errJSON(c, http.StatusUnauthorized, "unauthorized")
	}
	t, found, err := s.Store.GetGenerationTailored(userID, c.Param("id"))
	if err != nil {
		slog.Error("load generation failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	if !found {
		return errJSON(c, http.StatusNotFound, "no such generation")
	}
	return c.JSON(http.StatusOK, t)
}

// putGeneration applies the user's edits to a generation: the content is
// re-validated against the profile guardrail (rephrase your own history,
// never invent), re-normalized, re-rendered, and stored in place.
func (s *Server) putGeneration(c echo.Context) error {
	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		return errJSON(c, http.StatusUnauthorized, "unauthorized")
	}
	var t model.Tailored
	if err := c.Bind(&t); err != nil {
		return errJSON(c, http.StatusBadRequest, "invalid request body")
	}

	p, err := s.Store.LoadProfile(userID)
	if err != nil {
		slog.Error("load profile failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	if p == nil {
		return errJSON(c, http.StatusConflict, "no profile")
	}
	if err := model.ValidateTailored(*p, t); err != nil {
		return errJSON(c, http.StatusUnprocessableEntity, err.Error())
	}
	model.NormalizeTailored(&t)

	pdf, err := pdfgen.Render(*p, t, s.loadStyle(userID))
	if err != nil {
		slog.Error("render failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}

	found, err := s.Store.UpdateGenerationContent(userID, c.Param("id"), t, pdf)
	if err != nil {
		slog.Error("update generation failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	if !found {
		return errJSON(c, http.StatusNotFound, "no such generation")
	}
	return c.JSON(http.StatusOK, t)
}

func (s *Server) putGenerationPin(c echo.Context) error {
	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		return errJSON(c, http.StatusUnauthorized, "unauthorized")
	}
	var req struct {
		Pinned bool `json:"pinned"`
	}
	if err := c.Bind(&req); err != nil {
		return errJSON(c, http.StatusBadRequest, "invalid request body")
	}
	found, err := s.Store.SetGenerationPinned(userID, c.Param("id"), req.Pinned)
	if err != nil {
		slog.Error("pin generation failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	if !found {
		return errJSON(c, http.StatusNotFound, "no such generation")
	}
	return c.NoContent(http.StatusNoContent)
}

func (s *Server) putGenerationStatus(c echo.Context) error {
	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		return errJSON(c, http.StatusUnauthorized, "unauthorized")
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := c.Bind(&req); err != nil {
		return errJSON(c, http.StatusBadRequest, "invalid request body")
	}
	if !store.GenerationStatuses[req.Status] {
		return errJSON(c, http.StatusBadRequest, fmt.Sprintf("unknown status %q", req.Status))
	}
	found, err := s.Store.SetGenerationStatus(userID, c.Param("id"), req.Status)
	if err != nil {
		slog.Error("set generation status failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	if !found {
		return errJSON(c, http.StatusNotFound, "no such generation")
	}
	return c.NoContent(http.StatusNoContent)
}

func (s *Server) listGenerations(c echo.Context) error {
	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		return errJSON(c, http.StatusUnauthorized, "unauthorized")
	}
	list, err := s.Store.ListGenerations(userID)
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
	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		return errJSON(c, http.StatusUnauthorized, "unauthorized")
	}
	trends, total, err := s.Store.GapSummary(userID)
	if err != nil {
		slog.Error("gap summary failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, gapsResponse{Total: total, Trends: model.NonNil(trends)})
}

// generationParams are the writing knobs applied to every Tailor call.
type generationParams struct {
	Tone    string `json:"tone"`
	Summary string `json:"summary"`
	Bullets string `json:"bullets"`
}

func defaultGenerationParams() generationParams {
	return generationParams{Tone: "plain", Summary: "standard", Bullets: "full"}
}

func (g generationParams) validate() error {
	if g.Tone != "plain" && g.Tone != "confident" {
		return fmt.Errorf("unknown tone %q", g.Tone)
	}
	if g.Summary != "standard" && g.Summary != "short" && g.Summary != "none" {
		return fmt.Errorf("unknown summary %q", g.Summary)
	}
	if g.Bullets != "full" && g.Bullets != "lean" {
		return fmt.Errorf("unknown bullets %q", g.Bullets)
	}
	return nil
}

// options maps the stored params onto ai.TailorOptions, where defaults mean
// "add nothing to the prompt".
func (g generationParams) options() ai.TailorOptions {
	return ai.TailorOptions{Tone: g.Tone, Summary: g.Summary, Bullets: g.Bullets}
}

// loadGenerationParams resolves the user's saved writing knobs, falling back
// to defaults on absence or bad data, same discipline as loadStyle.
func (s *Server) loadGenerationParams(userID int64) generationParams {
	raw, err := s.Store.GetGenerationParams(userID)
	if err != nil || raw == nil {
		return defaultGenerationParams()
	}
	g := defaultGenerationParams()
	if json.Unmarshal(raw, &g) != nil || g.validate() != nil {
		return defaultGenerationParams()
	}
	return g
}

type settingsResponse struct {
	ResumeStyle   pdfgen.Style     `json:"resumeStyle"`
	EmailCopy     bool             `json:"emailCopy"`
	RecruiterAuto bool             `json:"recruiterAuto"`
	Generation    generationParams `json:"generation"`
}

// fileNumericID returns the 4-digit id that keeps download filenames apart.
func fileNumericID() int {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 1000 + int(time.Now().UnixMilli()%9000)
	}
	return 1000 + (int(b[0])<<8|int(b[1]))%9000
}

// loadStyle resolves the user's saved style, falling back to defaults on
// absence or bad data — read paths never fail over settings.
func (s *Server) loadStyle(userID int64) pdfgen.Style {
	raw, err := s.Store.GetResumeStyle(userID)
	if err != nil || raw == nil {
		return pdfgen.DefaultStyle()
	}
	var st pdfgen.Style
	if json.Unmarshal(raw, &st) != nil {
		return pdfgen.DefaultStyle()
	}
	return st.Normalized()
}

func (s *Server) getSettings(c echo.Context) error {
	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		return errJSON(c, http.StatusUnauthorized, "unauthorized")
	}
	emailCopy, err := s.Store.GetEmailCopy(userID)
	if err != nil {
		slog.Error("email copy setting lookup failed", "err", err)
		emailCopy = true
	}
	recruiterAuto, err := s.Store.GetRecruiterAuto(userID)
	if err != nil {
		slog.Error("recruiter auto setting lookup failed", "err", err)
	}
	return c.JSON(http.StatusOK, settingsResponse{
		ResumeStyle:   s.loadStyle(userID),
		EmailCopy:     emailCopy,
		RecruiterAuto: recruiterAuto,
		Generation:    s.loadGenerationParams(userID),
	})
}

func (s *Server) putSettings(c echo.Context) error {
	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		return errJSON(c, http.StatusUnauthorized, "unauthorized")
	}
	var req settingsResponse
	if err := c.Bind(&req); err != nil {
		return errJSON(c, http.StatusBadRequest, "invalid body")
	}
	if err := req.ResumeStyle.Validate(); err != nil {
		return errJSON(c, http.StatusBadRequest, err.Error())
	}
	// Absent knobs mean "leave at default" so a body without generation
	// stays valid.
	d := defaultGenerationParams()
	if req.Generation.Tone == "" {
		req.Generation.Tone = d.Tone
	}
	if req.Generation.Summary == "" {
		req.Generation.Summary = d.Summary
	}
	if req.Generation.Bullets == "" {
		req.Generation.Bullets = d.Bullets
	}
	if err := req.Generation.validate(); err != nil {
		return errJSON(c, http.StatusBadRequest, err.Error())
	}
	raw, err := json.Marshal(req.ResumeStyle)
	if err != nil {
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	if err := s.Store.SaveResumeStyle(userID, raw); err != nil {
		slog.Error("save settings failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	if err := s.Store.SaveEmailCopy(userID, req.EmailCopy); err != nil {
		slog.Error("save settings failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	genRaw, err := json.Marshal(req.Generation)
	if err != nil {
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	if err := s.Store.SaveGenerationParams(userID, genRaw); err != nil {
		slog.Error("save settings failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	if err := s.Store.SaveRecruiterAuto(userID, req.RecruiterAuto); err != nil {
		slog.Error("save settings failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, settingsResponse{ResumeStyle: req.ResumeStyle, EmailCopy: req.EmailCopy, RecruiterAuto: req.RecruiterAuto, Generation: req.Generation})
}

// sampleTailored builds a no-LLM stand-in from the profile itself so the
// preview shows real content in the chosen style without a generation.
func sampleTailored(p model.Profile) model.Tailored {
	items := p.Items
	if len(items) > 4 {
		items = items[:4]
	}
	var tItems []model.TItem
	for _, it := range items {
		bullets := it.Bullets
		if len(bullets) > 3 {
			bullets = bullets[:3]
		}
		var tb []model.TBullet
		for _, b := range bullets {
			tb = append(tb, model.TBullet{SourceBulletID: b.ID, Text: b.Text})
		}
		dates := it.StartDate
		if it.EndDate != "" {
			if dates != "" {
				dates += " – " + it.EndDate
			} else {
				dates = it.EndDate
			}
		}
		tItems = append(tItems, model.TItem{
			SourceID: it.ID, Title: it.Title, Organization: it.Organization,
			Dates: dates, Bullets: tb,
		})
	}
	return model.Tailored{
		TargetRole:     "Sample resume",
		Headline:       "Sample resume",
		SelectedSkills: p.Skills,
		Sections:       []model.TSection{{Title: "Experience", Items: tItems}},
	}
}

func (s *Server) getSettingsPreview(c echo.Context) error {
	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		return errJSON(c, http.StatusUnauthorized, "unauthorized")
	}
	p, err := s.Store.LoadProfile(userID)
	if err != nil {
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	if p == nil {
		return errJSON(c, http.StatusNotFound, "no profile yet")
	}
	pdf, err := pdfgen.Render(*p, sampleTailored(*p), s.loadStyle(userID))
	if err != nil {
		slog.Error("settings preview render failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	c.Response().Header().Set(echo.HeaderContentDisposition, `inline; filename="cvx-preview.pdf"`)
	return c.Blob(http.StatusOK, "application/pdf", pdf)
}

func (s *Server) deleteGeneration(c echo.Context) error {
	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		return errJSON(c, http.StatusUnauthorized, "unauthorized")
	}
	deleted, err := s.Store.DeleteGeneration(userID, c.Param("id"))
	if err != nil {
		slog.Error("delete generation failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	if !deleted {
		return errJSON(c, http.StatusNotFound, "unknown generation id")
	}
	return c.NoContent(http.StatusNoContent)
}

func (s *Server) getGenerationPDF(c echo.Context) error {
	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		return errJSON(c, http.StatusUnauthorized, "unauthorized")
	}
	id := c.Param("id")
	pdf, filename, err := s.Store.GetGenerationPDF(userID, id)
	if err != nil {
		slog.Error("get generation pdf failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	if pdf == nil {
		return errJSON(c, http.StatusNotFound, "unknown generation id")
	}
	// ?inline=1 renders in the browser (the in-app preview); default stays a
	// download.
	disposition := "attachment"
	if c.QueryParam("inline") == "1" {
		disposition = "inline"
	}
	c.Response().Header().Set(echo.HeaderContentDisposition, fmt.Sprintf(`%s; filename="%s"`, disposition, filename))
	return c.Blob(http.StatusOK, "application/pdf", pdf)
}

func (s *Server) getGenerationCoverPDF(c echo.Context) error {
	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		return errJSON(c, http.StatusUnauthorized, "unauthorized")
	}
	id := c.Param("id")
	pdf, filename, err := s.Store.GetGenerationCoverPDF(userID, id)
	if err != nil {
		slog.Error("get generation cover pdf failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	if pdf == nil {
		return errJSON(c, http.StatusNotFound, "no cover letter for this generation")
	}
	// ?inline=1 renders in the browser (the in-app preview); default stays a
	// download.
	disposition := "attachment"
	if c.QueryParam("inline") == "1" {
		disposition = "inline"
	}
	c.Response().Header().Set(echo.HeaderContentDisposition, fmt.Sprintf(`%s; filename="%s"`, disposition, filename))
	return c.Blob(http.StatusOK, "application/pdf", pdf)
}
