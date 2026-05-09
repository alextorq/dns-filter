package web

type LoginRequest struct {
	Login    string `json:"login" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type UserResponse struct {
	ID    uint   `json:"id"`
	Login string `json:"login"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type StatusResponse struct {
	Status string `json:"status"`
}
