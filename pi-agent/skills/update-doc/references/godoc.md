# Godoc

- Document exported API declarations and packages according to project conventions.
- Document unexported code when non-obvious behavior warrants it; do not add comments mechanically.
- Start declaration comments with the item name and use complete sentences.
- Keep comments short. Add details for errors, constraints, concurrency, or edge cases callers need.
- Do not use JSDoc-style tags or restate obvious signatures.

```go
// Package cache implements an in-memory cache with TTL expiration.
package cache

// NormalizeEmail lowercases and trims whitespace from email.
func NormalizeEmail(email string) string
```
