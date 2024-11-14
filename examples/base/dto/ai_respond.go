package dto

type QuestionRequest struct {
	Question string `json:"question"`
}

type TodoStep struct {
	ID          int    `json:"id"`
	Content     string `json:"content"`
	IsCompleted bool   `json:"is_completed"`
}

type TodoResponse struct {
	Question  string     `json:"question"`
	Steps     []TodoStep `json:"steps"`
	CreatedAt string     `json:"created_at"`
}
