package auth

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// BootstrapUserID is the user that owns the first API key (FLEETML_ADMIN_KEY)
// and every key created before users existed. It is a global admin.
var BootstrapUserID = uuid.MustParse("00000000-0000-4000-8000-000000000001")

// Roles in a project, from least to most.
const (
	RoleViewer   = "viewer"   // reads everything in the project
	RoleOperator = "operator" // also publishes models and runs deployments and experiments
	RoleAdmin    = "admin"    // also manages keys, alert sinks, signing, members and the project
)

// User is a person or service the API keys act as.
type User struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Admin     bool      `json:"admin"` // a global admin: every project, users and API keys
	CreatedAt time.Time `json:"created_at"`
}

// Member is a user's role in a project.
type Member struct {
	UserID   uuid.UUID `json:"user_id"`
	UserName string    `json:"user_name"`
	Role     string    `json:"role"`
}

// Principal is who a request acts as.
type Principal struct {
	User  User
	Key   string            // the API key's name
	Roles map[string]string // project ID -> role
}

// UserStore persists users and their roles.
type UserStore interface {
	CreateUser(ctx context.Context, name string, admin bool) (*User, error)
	ListUsers(ctx context.Context) ([]User, error)
	SetRole(ctx context.Context, userID, projectID uuid.UUID, role string) error
	RemoveRole(ctx context.Context, userID, projectID uuid.UUID) error
	Members(ctx context.Context, projectID uuid.UUID) ([]Member, error)
	// PrincipalForKey loads the user of an API key and the user's roles.
	PrincipalForKey(ctx context.Context, apiKeyID uuid.UUID) (*Principal, error)
}

// UserService manages users and roles.
type UserService struct {
	store UserStore
}

// NewUserService creates a UserService.
func NewUserService(store UserStore) *UserService { return &UserService{store: store} }

var userName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._@-]{0,99}$`)

// Create adds a user.
func (s *UserService) Create(ctx context.Context, name string, admin bool) (*User, error) {
	name = strings.TrimSpace(name)
	if !userName.MatchString(name) {
		return nil, fmt.Errorf("%w: name: 1 to 100 letters, digits, spaces or . _ @ -", ErrInvalidInput)
	}
	return s.store.CreateUser(ctx, name, admin)
}

// List returns all users.
func (s *UserService) List(ctx context.Context) ([]User, error) { return s.store.ListUsers(ctx) }

// SetRole gives a user a role in a project (replacing any other).
func (s *UserService) SetRole(ctx context.Context, userID, projectID uuid.UUID, role string) error {
	switch role {
	case RoleViewer, RoleOperator, RoleAdmin:
	default:
		return fmt.Errorf("%w: role must be viewer, operator or admin", ErrInvalidInput)
	}
	return s.store.SetRole(ctx, userID, projectID, role)
}

// RemoveRole takes a user out of a project.
func (s *UserService) RemoveRole(ctx context.Context, userID, projectID uuid.UUID) error {
	return s.store.RemoveRole(ctx, userID, projectID)
}

// Members lists a project's members.
func (s *UserService) Members(ctx context.Context, projectID uuid.UUID) ([]Member, error) {
	return s.store.Members(ctx, projectID)
}

// PrincipalForKey loads who an API key acts as.
func (s *UserService) PrincipalForKey(ctx context.Context, apiKeyID uuid.UUID) (*Principal, error) {
	return s.store.PrincipalForKey(ctx, apiKeyID)
}
