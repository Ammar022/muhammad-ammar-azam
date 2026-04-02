// Package controller contains the HTTP handler for the chat module.
// It sits at the outermost layer of the architecture: it handles HTTP
// concerns (parsing, validation, serialisation) and delegates all
// business logic to the domain service.
package controller

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/microcosm-cc/bluemonday"

	"github.com/Ammar022/secure-ai-chat-backend/internal/chat/domain"
	"github.com/Ammar022/secure-ai-chat-backend/internal/chat/dto"
	"github.com/Ammar022/secure-ai-chat-backend/internal/shared/auth"
	apperrors "github.com/Ammar022/secure-ai-chat-backend/internal/shared/errors"
	"github.com/Ammar022/secure-ai-chat-backend/internal/shared/middleware"
	"github.com/Ammar022/secure-ai-chat-backend/internal/shared/response"
)

// ChatController handles all HTTP requests for the /api/v1/chat route group.
type ChatController struct {
	service   *domain.ChatService
	validate  *validator.Validate
	sanitizer *bluemonday.Policy
}

// NewChatController creates a ChatController.
// The bluemonday strict policy strips all HTML/JavaScript, defending against
// stored XSS attacks where a question could later be rendered in a UI.
func NewChatController(service *domain.ChatService) *ChatController {
	return &ChatController{
		service:   service,
		validate:  validator.New(),
		sanitizer: bluemonday.StrictPolicy(),
	}
}

// Routes registers all chat endpoints on the given chi router.
// All routes require the JWT + anti-replay middleware (applied at the parent level).
func (c *ChatController) Routes(r chi.Router) {
	r.Post("/", c.sendMessage)
	r.Get("/", c.listMessages)
	r.Get("/{id}", c.getMessage)
}

// sendMessage handles POST /api/v1/chat/messages
//
//	@Summary		Send a message to the AI
//	@Description	Submits a question, validates quota (free tier or subscription bundle), calls the mocked AI, stores and returns the result. Deducts from the subscription bundle with the most recent remaining quota.
//	@Tags			chat
//	@Accept			json
//	@Produce		json
//	@Param			X-Request-Timestamp	header		string	true	"Unix timestamp in seconds (e.g. 1711234567) — must be within ±5 min of server time"
//	@Param			X-Nonce				header		string	true	"Unique string per request, e.g. a UUID (rejected if reused — anti-replay)"
//	@Param			body				body		dto.SendMessageRequest	true	"Question payload"
//	@Success		201	{object}		response.Envelope{data=messageResponse}	"Message stored and AI response returned"
//	@Failure		400	{object}		response.Envelope	"Invalid input"
//	@Failure		402	{object}		response.Envelope	"No quota available — subscribe to continue"
//	@Failure		422	{object}		response.Envelope	"Validation error"
//	@Failure		401	{object}		response.Envelope	"Unauthorized"
//	@Failure		429	{object}		response.Envelope	"Rate limited"
//	@Security		BearerAuth
//	@Router			/api/v1/chat/messages [post]
func (c *ChatController) sendMessage(w http.ResponseWriter, r *http.Request) {
	claims := auth.MustClaimsFromContext(r.Context())

	// Prevent mass-assignment: use a strict decoder that rejects unknown fields
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req dto.SendMessageRequest
	if err := dec.Decode(&req); err != nil {
		response.ErrorWithDetails(w, apperrors.ErrInvalidInput, err.Error())
		return
	}

	// Schema-based validation
	if err := c.validate.Struct(req); err != nil {
		response.ErrorWithDetails(w, apperrors.ErrValidation, formatValidationErrors(err))
		return
	}

	// Sanitize against XSS and injection before passing to the domain
	req.Question = c.sanitizer.Sanitize(req.Question)
	if req.Question == "" {
		response.Error(w, apperrors.ValidationError("question contains only unsafe content"))
		return
	}

	msg, err := c.service.SendMessage(
		r.Context(),
		claims.InternalUserID,
		req.Question,
		getClientIP(r),
		middleware.RequestIDFromContext(r.Context()),
	)
	if err != nil {
		response.Error(w, err)
		return
	}

	response.Created(w, toMessageResponse(msg))
}

// listMessages handles GET /api/v1/chat/messages?page=1&per_page=20
//
//	@Summary		List chat messages
//	@Description	Returns a paginated list of the authenticated user's chat history, newest first.
//	@Tags			chat
//	@Produce		json
//	@Param			X-Request-Timestamp	header		string	true	"Unix timestamp in seconds (e.g. 1711234567)"
//	@Param			X-Nonce				header		string	true	"Unique string per request, e.g. a UUID"
//	@Param			page				query		int		false	"Page number (default: 1)"
//	@Param			per_page			query		int		false	"Items per page, 1–100 (default: 20)"
//	@Success		200	{object}		response.Envelope{data=[]messageResponse}	"Paginated messages"
//	@Failure		401	{object}		response.Envelope	"Unauthorized"
//	@Security		BearerAuth
//	@Router			/api/v1/chat/messages [get]
func (c *ChatController) listMessages(w http.ResponseWriter, r *http.Request) {
	claims := auth.MustClaimsFromContext(r.Context())

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	msgs, total, err := c.service.ListMessages(r.Context(), claims.InternalUserID, page, perPage)
	if err != nil {
		response.Error(w, err)
		return
	}

	items := make([]messageResponse, 0, len(msgs))
	for _, m := range msgs {
		items = append(items, toMessageResponse(m))
	}

	totalPages := int(math.Ceil(float64(total) / float64(perPage)))
	response.Paginated(w, items, response.PaginationMeta{
		Page:       page,
		PerPage:    perPage,
		TotalItems: total,
		TotalPages: totalPages,
	})
}

// getMessage handles GET /api/v1/chat/messages/{id}
//
//	@Summary		Get a single chat message
//	@Description	Returns one chat message by ID. Users can only retrieve their own messages.
//	@Tags			chat
//	@Produce		json
//	@Param			X-Request-Timestamp	header		string	true	"Unix timestamp in seconds (e.g. 1711234567)"
//	@Param			X-Nonce				header		string	true	"Unique string per request, e.g. a UUID"
//	@Param			id					path		string	true	"Message UUID (e.g. 550e8400-e29b-41d4-a716-446655440000)"
//	@Success		200	{object}		response.Envelope{data=messageResponse}	"Chat message"
//	@Failure		400	{object}		response.Envelope	"Invalid UUID"
//	@Failure		403	{object}		response.Envelope	"Forbidden"
//	@Failure		404	{object}		response.Envelope	"Not found"
//	@Failure		401	{object}		response.Envelope	"Unauthorized"
//	@Security		BearerAuth
//	@Router			/api/v1/chat/messages/{id} [get]
func (c *ChatController) getMessage(w http.ResponseWriter, r *http.Request) {
	claims := auth.MustClaimsFromContext(r.Context())

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		response.Error(w, apperrors.ErrInvalidInput)
		return
	}

	msg, err := c.service.GetMessage(r.Context(), claims.InternalUserID, id)
	if err != nil {
		response.Error(w, err)
		return
	}

	response.Success(w, toMessageResponse(msg))
}

// ── Response shape ────────────────────────────────────────────────────────────

// messageResponse is the public-facing representation of a ChatMessage.
// We intentionally omit internal fields (ip_address, request_id) from
// the standard user-facing response.
type messageResponse struct {
	ID               string  `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Question         string  `json:"question" example:"What is quantum computing and how does it work?"`
	Answer           string  `json:"answer" example:"This is a simulated AI response to your question."`
	PromptTokens     int     `json:"prompt_tokens" example:"12"`
	CompletionTokens int     `json:"completion_tokens" example:"85"`
	TotalTokens      int     `json:"total_tokens" example:"97"`
	ResponseTimeMs   int64   `json:"response_time_ms" example:"743"`
	SubscriptionID   *string `json:"subscription_id,omitempty" example:"a1b2c3d4-e5f6-7890-abcd-ef1234567890"`
	CreatedAt        string  `json:"created_at" example:"2024-03-21T10:30:00Z"`
}

func toMessageResponse(m *domain.ChatMessage) messageResponse {
	r := messageResponse{
		ID:               m.ID.String(),
		Question:         m.Question,
		Answer:           m.Answer,
		PromptTokens:     m.PromptTokens,
		CompletionTokens: m.CompletionTokens,
		TotalTokens:      m.TotalTokens,
		ResponseTimeMs:   m.ResponseTimeMs,
		CreatedAt:        m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if m.SubscriptionID != nil {
		s := m.SubscriptionID.String()
		r.SubscriptionID = &s
	}
	return r
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// formatValidationErrors converts go-playground/validator errors into a
// client-friendly map of field → error message.
func formatValidationErrors(err error) map[string]string {
	errs := make(map[string]string)
	if ve, ok := err.(validator.ValidationErrors); ok {
		for _, fe := range ve {
			switch fe.Field() {
			case "Question":
				switch fe.Tag() {
				case "required":
					errs["Question"] = "question is required"
				case "min":
					errs["Question"] = "question must be at least 1 character long"
				case "max":
					errs["Question"] = "question must be at most 4000 characters long"
				default:
					errs["Question"] = "invalid question"
				}
			case "Page":
				if fe.Tag() == "min" {
					errs["Page"] = "page must be at least 1"
				} else {
					errs["Page"] = "invalid page"
				}
			case "PerPage":
				switch fe.Tag() {
				case "min":
					errs["PerPage"] = "per_page must be at least 1"
				case "max":
					errs["PerPage"] = "per_page must be at most 100"
				default:
					errs["PerPage"] = "invalid per_page"
				}
			default:
				errs[fe.Field()] = fe.Tag()
			}
		}
	}
	return errs
}

// getClientIP extracts the real client IP from the request.
func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	return r.RemoteAddr
}
