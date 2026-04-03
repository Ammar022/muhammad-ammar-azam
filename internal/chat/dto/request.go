package dto

type SendMessageRequest struct {
	Question string `json:"question" validate:"required,min=1,max=4000"`
}

type ListMessagesQuery struct {
	Page    int `schema:"page" validate:"min=1"`
	PerPage int `schema:"per_page" validate:"min=1,max=100"`
}
