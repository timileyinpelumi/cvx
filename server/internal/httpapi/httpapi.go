// Package httpapi wires the cvx HTTP contract onto an echo.Echo: digitize,
// tailor, render, persist, and (best-effort) email a tailored resume PDF.
// Handlers are thin — all domain logic lives in ai, pdfgen, and store, which
// are already unit-tested; this package only translates HTTP <-> those calls.
package httpapi

import (
	"crypto/rand"
	"encoding/base64"
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
	// LifecycleMail sends an account email (welcome, farewell). Optional:
	// nil means the deployment has no mail provider configured.
	LifecycleMail func(kind, to, name string)
	// RecruiterMail sends the forwardable recruiter-facing email instead of
	// the private notification when a generation requests it.
	RecruiterMail func(to string, re model.RecruiterEmail, name string, pdf []byte, filename string, coverPDF []byte, coverFilename string) (bool, error)
	Auth          *auth.Auth
}

func (s *Server) Register(e *echo.Echo) {
	// /auth/* is registered directly on e, outside the /api group, so it
	// never runs through the group's own auth middleware below — these
	// routes are how a session gets established in the first place.
	// Expensive routes carry their own tighter limiter and a per-account
	// concurrency gate on top of the global one.
	costly := costlyMiddleware()
	signIn := authMiddleware()

	e.GET("/auth/providers", s.Auth.ListProviders)
	e.GET("/auth/:provider/start", s.Auth.AuthStart, signIn)
	e.GET("/auth/:provider/callback", s.Auth.AuthCallback, signIn)

	// /api/logout is also registered outside the guarded group: a bad or
	// expired session cookie must still be clearable, not stuck behind the
	// 401 it would itself cause.
	e.POST("/api/logout", s.Auth.Logout)

	api := e.Group("/api")
	api.Use(s.Auth.Middleware)
	api.GET("/me", s.Auth.GetMe)
	api.DELETE("/account", s.deleteAccount)
	api.GET("/profile", s.getProfile)
	api.GET("/profile/edits", s.getProfileEdits)
	api.PUT("/profile", s.putProfile)
	api.POST("/profile", s.postProfile, costly...)
	api.POST("/profile/extend", s.postProfileExtend, costly...)
	api.GET("/profile/history", s.getProfileHistory)
	api.POST("/profile/restore", s.postProfileRestore)
	api.GET("/profile/skills", s.getSkills)
	api.PUT("/profile/skills", s.putSkills)
	api.POST("/generate", s.postGenerate, costly...)
	api.GET("/generations", s.listGenerations)
	api.GET("/gaps", s.getGaps)
	api.GET("/settings", s.getSettings)
	api.PUT("/settings", s.putSettings)
	api.GET("/settings/preview", s.getSettingsPreview)
	api.DELETE("/generations/:id", s.deleteGeneration)
	api.GET("/generations/:id/tailored", s.getGenerationTailored)
	api.GET("/generations/:id/provenance", s.getGenerationProvenance)
	api.POST("/generations/:id/bullet", s.rewriteBullet, costly...)
	api.POST("/generations/:id/preview", s.previewGeneration, costly...)
	api.GET("/generations/:id/available", s.getAvailableContent)
	api.POST("/generations/:id/followup", s.generateFollowUp, costly...)
	api.PUT("/generations/:id", s.putGeneration, costly...)
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

// Lifecycle email kinds, so the wiring in main stays a switch on a constant
// rather than on a loose string.
const (
	MailWelcome  = "welcome"
	MailFarewell = "farewell"
)

func errJSON(c echo.Context, status int, msg string) error {
	return c.JSON(status, map[string]string{"error": msg})
}

// deleteAccount closes the signed-in account and ends the session. The
// account row is tombstoned, not removed, so signing in again with the same
// identity starts a fresh account.
func (s *Server) deleteAccount(c echo.Context) error {
	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		return errJSON(c, http.StatusUnauthorized, "unauthorized")
	}
	// Read the address before closing the account: afterwards the row's
	// email is tombstoned and no longer deliverable.
	var email, name string
	if u, err := s.Store.GetUser(userID); err != nil {
		slog.Warn("delete account: user lookup failed", "err", err)
	} else if u != nil {
		email, name = u.Email, u.Name
	}

	if err := s.Store.SoftDeleteUser(userID); err != nil {
		slog.Error("delete account failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	if s.LifecycleMail != nil && email != "" {
		go s.LifecycleMail(MailFarewell, email, name)
	}
	return s.Auth.Logout(c)
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

// getProfileEdits returns the handful of facts the editor owns. Not the
// whole profile: work history is read from the uploaded resume and extended
// by note, and hand-editing it here would be a CV manager, which cvx is not.
func (s *Server) getProfileEdits(c echo.Context) error {
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
	return c.JSON(http.StatusOK, model.EditsFrom(*p))
}

// putProfile saves those facts. Everything else in the profile is left
// exactly as it was, and SaveProfile snapshots the outgoing version, so a
// bad edit is one restore away.
func (s *Server) putProfile(c echo.Context) error {
	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		return errJSON(c, http.StatusUnauthorized, "unauthorized")
	}
	var edits model.ProfileEdits
	if err := c.Bind(&edits); err != nil {
		return errJSON(c, http.StatusBadRequest, "invalid request body")
	}
	if strings.TrimSpace(edits.Name) == "" {
		return errJSON(c, http.StatusBadRequest, "your name is required")
	}

	stored, err := s.Store.LoadProfile(userID)
	if err != nil {
		slog.Error("load profile failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	if stored == nil {
		return errJSON(c, http.StatusConflict, "no profile")
	}

	updated := model.ApplyEdits(*stored, edits)
	if err := s.Store.SaveProfile(userID, updated); err != nil {
		slog.Error("save profile failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, model.EditsFrom(updated))
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
	ID             string             `json:"id"`
	Filename       string             `json:"filename"`
	Gaps           []model.Gap        `json:"gaps"`
	WhatChanged    []string           `json:"whatChanged"`
	Emailed        bool               `json:"emailed"`
	CoverFilename  string             `json:"coverFilename"`
	CoverLetter    bool               `json:"coverLetter"`
	RecruiterEmail bool               `json:"recruiterEmail"`
	Fit            *model.Fit         `json:"fit,omitempty"`
	Coverage       *model.Coverage    `json:"coverage,omitempty"`
	ProseWarnings  []model.Ungrounded `json:"proseWarnings,omitempty"`
	// PageFill is how much of the page the resume covers, 0 to 1, and
	// PageAdvice is what would fill it when it fell short.
	PageFill   float64  `json:"pageFill"`
	PageAdvice []string `json:"pageAdvice,omitempty"`
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

	posting, err := ai.ParsePosting(ctx, s.LLM, req.RoleInput)
	if err != nil {
		if errors.Is(err, ai.ErrNotJobInput) {
			slog.Info("job input rejected", "err", err)
			return errJSON(c, http.StatusUnprocessableEntity, msgNotJobInput)
		}
		slog.Error("parse posting failed", "err", err)
		return errJSON(c, http.StatusBadGateway, err.Error())
	}

	tailored, err := ai.TailorWithOptions(ctx, s.LLM, *p, posting, s.loadGenerationParams(userID).options())
	if err != nil {
		slog.Error("tailor failed", "err", err)
		return errJSON(c, http.StatusBadGateway, err.Error())
	}
	// Production clamps to the content standard; the eval harness deliberately
	// judges the raw Tailor output, so this lives here and not inside ai.Tailor.
	model.NormalizeTailored(&tailored)

	style := s.loadStyle(userID)
	pdf, layout, err := pdfgen.RenderWithLayout(*p, tailored, style)
	if err == nil {
		// Read our own output back with the extractor an ATS would use. A
		// failure here means the page does not say what the pipeline thinks
		// it says, which is a rendering bug, not a reason to withhold the
		// resume — so it is logged loudly and the download still happens.
		if issues := pdfgen.Verify(pdf, *p, tailored); len(issues) > 0 {
			slog.Error("rendered resume failed verification", "issues", issues)
		}
	}
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
	// prose collects the free-text artifacts for the grounding audit below,
	// keyed by the label a warning names.
	prose := map[string]string{}
	if req.CoverLetter {
		if cl, err := ai.CoverLetter(ctx, s.LLM, *p, posting); err != nil {
			slog.Error("cover letter failed", "err", err)
		} else if rendered, err := pdfgen.RenderCoverLetter(*p, tailored.TargetRole, cl, style); err != nil {
			slog.Error("cover letter failed", "err", err)
		} else {
			coverPDF = rendered
			coverFilename = model.CoverFilename(p.Name, tailored.TargetRole, fileID)
			prose[ai.ArtifactCoverLetter] = strings.Join(cl.Paragraphs, "\n\n")
		}
	}

	meta, err := s.Store.SaveGeneration(userID, tailored, pdf, filename, coverPDF, coverFilename, fingerprint)
	if err == nil {
		if err := s.Store.SavePosting(userID, meta.ID, posting); err != nil {
			slog.Warn("save posting failed", "err", err, "id", meta.ID)
		}
	}
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
					re, err := ai.RecruiterEmail(ctx, s.LLM, *p, posting)
					if err != nil {
						slog.Error("recruiter email failed", "err", err)
					} else {
						prose[ai.ArtifactEmail] = re.Subject + "\n\n" + strings.Join(re.Paragraphs, "\n\n")
						ok, err := s.RecruiterMail(u.Email, re, p.Name, pdf, filename, coverPDF, coverFilename)
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

	// Advisory grounding audit over the free prose. The resume is guarded by
	// ids; these two are not, so they get checked against the profile before
	// the user forwards them anywhere.
	var proseWarnings []model.Ungrounded
	if len(prose) > 0 {
		if found, err := ai.AuditGrounding(ctx, s.LLM, *p, prose); err != nil {
			slog.Warn("grounding audit failed", "err", err)
		} else if len(found) > 0 {
			slog.Warn("generated prose has unsupported claims", "count", len(found))
			proseWarnings = found
		}
	}

	pageAdvice := model.PageAdvice(*p, layout.Fill, len(layout.Trimmed))
	if len(pageAdvice) > 0 {
		slog.Info("resume did not fill the page", "fill", layout.Fill, "advice", len(pageAdvice))
	}

	coverage := model.CoverageOf(posting, tailored)
	fit := model.FitOf(posting, tailored, coverage)

	return c.JSON(http.StatusOK, generateResponse{
		ID:             meta.ID,
		Filename:       meta.Filename,
		Gaps:           model.NonNil(tailored.Gaps),
		WhatChanged:    model.NonNil(tailored.WhatChanged),
		Emailed:        emailed,
		CoverFilename:  coverFilename,
		CoverLetter:    coverFilename != "",
		RecruiterEmail: recruiterSent,
		Fit:            &fit,
		Coverage:       &coverage,
		ProseWarnings:  proseWarnings,
		PageFill:       layout.Fill,
		PageAdvice:     pageAdvice,
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

// provenanceEntry is where one line on the resume came from: the profile
// bullet it cites, and the item that bullet belongs to.
type provenanceEntry struct {
	Original     string `json:"original"`
	ItemTitle    string `json:"itemTitle"`
	Organization string `json:"organization"`
}

// getGenerationProvenance returns, per cited profile bullet id, the original
// wording from the profile. The tailored output already carries the ids; the
// guardrail already refuses anything else. This is what makes that visible:
// the user can see every line on the page traced back to something they
// wrote, which is the whole basis for trusting the output.
func (s *Server) getGenerationProvenance(c echo.Context) error {
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
		return errJSON(c, http.StatusConflict, "no profile")
	}
	t, found, err := s.Store.GetGenerationTailored(userID, c.Param("id"))
	if err != nil {
		slog.Error("load generation failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	if !found {
		return errJSON(c, http.StatusNotFound, "no such generation")
	}

	cited := map[string]bool{}
	for _, sec := range t.Sections {
		for _, it := range sec.Items {
			for _, b := range it.Bullets {
				cited[b.SourceBulletID] = true
			}
		}
	}

	out := map[string]provenanceEntry{}
	for _, item := range p.Items {
		for _, b := range item.Bullets {
			if cited[b.ID] {
				out[b.ID] = provenanceEntry{
					Original:     b.Text,
					ItemTitle:    item.Title,
					Organization: item.Organization,
				}
			}
		}
	}
	return c.JSON(http.StatusOK, out)
}

// followUpAfterDays is how long an application waits before a nudge is
// reasonable. Under a week is pushy; a month is too late to be useful.
const followUpAfterDays = 7

// generateFollowUp drafts the nudge for an application that has gone quiet.
// It refuses on anything not actually sent, and on anything sent too
// recently, because the tracker exists to tell the user when to follow up,
// not to let them do it on day one.
func (s *Server) generateFollowUp(c echo.Context) error {
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
		return errJSON(c, http.StatusConflict, "no profile")
	}

	id := c.Param("id")
	list, err := s.Store.ListGenerations(userID)
	if err != nil {
		slog.Error("list generations failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	var meta *store.GenerationMeta
	for i := range list {
		if list[i].ID == id {
			meta = &list[i]
		}
	}
	if meta == nil {
		return errJSON(c, http.StatusNotFound, "no such generation")
	}
	if meta.Status != store.StatusSent {
		return errJSON(c, http.StatusConflict, "only an application marked sent can be followed up")
	}

	days := daysSince(meta.StatusAt)
	if days < followUpAfterDays {
		return errJSON(c, http.StatusConflict, "too soon to follow up")
	}

	posting := model.Posting{Title: meta.TargetRole, Raw: meta.TargetRole}
	if stored, err := s.Store.GetPosting(userID, id); err != nil {
		slog.Warn("load posting failed", "err", err)
	} else if stored != nil {
		posting = *stored
	}

	re, err := ai.FollowUp(c.Request().Context(), s.LLM, *p, posting, days)
	if err != nil {
		slog.Error("follow up failed", "err", err)
		return errJSON(c, http.StatusBadGateway, err.Error())
	}
	return c.JSON(http.StatusOK, re)
}

// daysSince returns whole days between an RFC3339 stamp and now, or 0 when
// the stamp is missing or unparseable.
func daysSince(stamp string) int {
	t, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		return 0
	}
	return int(time.Since(t).Hours() / 24)
}

// previewResponse carries a rendered but unsaved resume: the PDF inline as
// base64 so one request answers both "what does it look like" and "does it
// fit", plus what the fit loop had to do to land it on a page.
type previewResponse struct {
	PDF        string   `json:"pdf"`
	Fill       float64  `json:"fill"`
	Trimmed    []string `json:"trimmed"`
	Stretched  bool     `json:"stretched"`
	PageAdvice []string `json:"pageAdvice,omitempty"`
}

// previewGeneration renders the working copy without storing anything, so
// the editor can show the real page while it is being edited. Editing blind
// and finding out at save time is the whole problem this removes.
func (s *Server) previewGeneration(c echo.Context) error {
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

	pdf, layout, err := pdfgen.RenderWithLayout(*p, t, s.loadStyle(userID))
	if err != nil {
		slog.Error("preview render failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, previewResponse{
		PDF:        base64.StdEncoding.EncodeToString(pdf),
		Fill:       layout.Fill,
		Trimmed:    model.NonNil(layout.Trimmed),
		Stretched:  layout.Stretched,
		PageAdvice: model.PageAdvice(*p, layout.Fill, len(layout.Trimmed)),
	})
}

// availableBullet and availableItem are profile content that is NOT on the
// resume: what the tailor left out, offered back to the user. The ids are
// the profile's own, so adding them keeps the citation guardrail satisfied.
type availableBullet struct {
	SourceBulletID string `json:"sourceBulletId"`
	Text           string `json:"text"`
}

type availableItem struct {
	SourceID     string            `json:"sourceId"`
	Kind         string            `json:"kind"`
	Title        string            `json:"title"`
	Organization string            `json:"organization"`
	Dates        string            `json:"dates"`
	OnResume     bool              `json:"onResume"`
	Bullets      []availableBullet `json:"bullets"`
}

type availableResponse struct {
	Items          []availableItem `json:"items"`
	Skills         []string        `json:"skills"`
	Certifications []string        `json:"certifications"`
	Languages      []string        `json:"languages"`
	Interests      []string        `json:"interests"`
}

// getAvailableContent lists what the profile holds that this resume does not
// use yet. The tailor deliberately over-selects and the renderer trims to the
// page, which means material the model skipped is otherwise invisible and
// unreachable: this is what makes the editor additive instead of only
// subtractive.
func (s *Server) getAvailableContent(c echo.Context) error {
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
		return errJSON(c, http.StatusConflict, "no profile")
	}
	t, found, err := s.Store.GetGenerationTailored(userID, c.Param("id"))
	if err != nil {
		slog.Error("load generation failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	if !found {
		return errJSON(c, http.StatusNotFound, "no such generation")
	}

	usedItems, usedBullets := map[string]bool{}, map[string]bool{}
	for _, sec := range t.Sections {
		for _, it := range sec.Items {
			usedItems[it.SourceID] = true
			for _, b := range it.Bullets {
				usedBullets[b.SourceBulletID] = true
			}
		}
	}

	out := availableResponse{Items: []availableItem{}}
	for _, item := range p.Items {
		avail := availableItem{
			SourceID:     item.ID,
			Kind:         item.Kind,
			Title:        item.Title,
			Organization: item.Organization,
			Dates:        item.DateRange(),
			OnResume:     usedItems[item.ID],
			Bullets:      []availableBullet{},
		}
		for _, b := range item.Bullets {
			if usedBullets[b.ID] {
				continue
			}
			avail.Bullets = append(avail.Bullets, availableBullet{SourceBulletID: b.ID, Text: b.Text})
		}
		// An item already on the resume with every bullet used has nothing
		// left to offer.
		if avail.OnResume && len(avail.Bullets) == 0 {
			continue
		}
		out.Items = append(out.Items, avail)
	}

	out.Skills = model.NonNil(p.Skills)
	out.Certifications = model.NonNil(certificationNames(*p))
	out.Languages = model.NonNil(p.Languages)
	out.Interests = model.NonNil(p.Interests)
	return c.JSON(http.StatusOK, out)
}

func certificationNames(p model.Profile) []string {
	out := make([]string, 0, len(p.Certifications))
	for _, c := range p.Certifications {
		out = append(out, c.Name)
	}
	return out
}

type rewriteBulletRequest struct {
	BulletID    string `json:"bulletId"`
	Current     string `json:"current"`
	Instruction string `json:"instruction"`
}

// rewriteBullet regenerates one line without re-rolling the resume. The
// profile bullet it cites is the ground truth and is looked up here rather
// than trusted from the request, so a rewrite can never be pointed at
// something the user did not write.
func (s *Server) rewriteBullet(c echo.Context) error {
	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		return errJSON(c, http.StatusUnauthorized, "unauthorized")
	}
	var req rewriteBulletRequest
	if err := c.Bind(&req); err != nil {
		return errJSON(c, http.StatusBadRequest, "bad request")
	}

	p, err := s.Store.LoadProfile(userID)
	if err != nil {
		slog.Error("load profile failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	if p == nil {
		return errJSON(c, http.StatusConflict, "no profile")
	}

	var source model.Bullet
	var itemTitle, organization string
	for _, item := range p.Items {
		for _, b := range item.Bullets {
			if b.ID == req.BulletID {
				source, itemTitle, organization = b, item.Title, item.Organization
			}
		}
	}
	if source.ID == "" {
		return errJSON(c, http.StatusNotFound, "no such profile bullet")
	}

	posting := model.Posting{}
	if stored, err := s.Store.GetPosting(userID, c.Param("id")); err != nil {
		slog.Warn("load posting failed", "err", err)
	} else if stored != nil {
		posting = *stored
	}

	text, err := ai.RewriteBullet(c.Request().Context(), s.LLM, source, itemTitle, organization, posting, req.Current, req.Instruction)
	if err != nil {
		slog.Error("rewrite bullet failed", "err", err)
		return errJSON(c, http.StatusBadGateway, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"text": text})
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
	if err == nil {
		if issues := pdfgen.Verify(pdf, *p, t); len(issues) > 0 {
			slog.Error("edited resume failed verification", "issues", issues)
		}
	}
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
