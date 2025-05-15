package auth

import (
	"context"
)

// contextKey is a custom type for context keys to prevent collisions
type contextKey string

// userKey is the key used to store the user in the context
const userKey = contextKey("user")

// SetUserInContext stores the user in the context
func SetUserInContext(ctx context.Context, user *User) context.Context {
	return context.WithValue(ctx, userKey, user)
}

// GetUserFromContext retrieves the user from the context
func GetUserFromContext(ctx context.Context) *User {
	value := ctx.Value(userKey)
	if value == nil {
		return nil
	}
	if user, ok := value.(*User); ok {
		return user
	}
	return nil
}

// IsAuthenticated checks if a user is authenticated in the context
func IsAuthenticated(ctx context.Context) bool {
	return GetUserFromContext(ctx) != nil
}

// IsAdmin checks if the authenticated user is an admin
func IsAdmin(ctx context.Context) bool {
	user := GetUserFromContext(ctx)
	return user != nil && user.Role == "admin"
}