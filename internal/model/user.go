package model

type User struct {
	ID             string   `json:"id"`
	FullName       string   `json:"fullName"`
	Email          string   `json:"email"`
	Password       string   `json:"-"`
	Roles          []string `json:"roles"`
	IsActive       bool     `json:"isActive"`
	TargetLanguage string   `json:"targetLanguage"`
}

type RegisterRequest struct {
	FullName string `json:"fullName"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}
