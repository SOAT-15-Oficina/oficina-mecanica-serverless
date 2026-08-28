package domain

import "github.com/google/uuid"

type UserRole string

const (
	UserRoleAdmin    UserRole = "admin"
	UserRoleEmployee UserRole = "employee"
)

// Espelha a tabela `users`, cujo schema e de propriedade do
// oficina-mecanica-monolith (database/migrations). Esta funcao nao roda
// migration nenhuma: ela le e escreve numa tabela que ja existe, e cujo `id` e
// alvo de chave estrangeira em work_orders e work_order_status_history.
type User struct {
	ID           uuid.UUID `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         UserRole  `json:"role"`
}
