package dto

type TodoStep struct {
	ID          int    `json:"id"`
	Content     string `json:"content"`
	IsCompleted bool   `json:"is_completed"`
}

type TodoResponse struct {
	Steps     []TodoStep `json:"steps"`
	CreatedAt string     `json:"created_at"`
}
