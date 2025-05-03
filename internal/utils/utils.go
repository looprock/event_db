package utils

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"bytes"
	"io"
	"log"
	"mime/multipart"
	"mime/quotedprintable"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jaytaylor/html2text"
	"golang.org/x/crypto/bcrypt"
)

// GenerateRandomString creates a random string of specified length
func GenerateRandomString(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes)[:length], nil
}

// GenerateEmailAddress creates a random email address for the given domain
func GenerateEmailAddress(randomLength int, domain string) (string, error) {
	random, err := GenerateRandomString(randomLength)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s@%s", strings.ToLower(random), domain), nil
}

// HashPassword creates a bcrypt hash of the password
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// CheckPassword compares a password against a hash
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// GenerateJWT creates a new JWT token for a user
func GenerateJWT(userID int64, email, role string, secret string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": userID,
		"email":   email,
		"role":    role,
		"exp":     time.Now().Add(24 * time.Hour).Unix(),
	})
	return token.SignedString([]byte(secret))
}

// GenerateRegistrationToken creates a new registration token
func GenerateRegistrationToken() (string, error) {
	return GenerateRandomString(64)
}

// ValidateEndpointURL ensures the endpoint URL is valid and has required format
func ValidateEndpointURL(url string) bool {
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
}

// SanitizeEmail removes whitespace and converts to lowercase
func SanitizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// IsValidRole checks if a role is valid
func IsValidRole(role string) bool {
	return role == "admin" || role == "user"
}

// cleanMessageContent removes unwanted signatures and normalizes content
func cleanMessageContent(content string) string {
	log.Printf("Cleaning content (before): %q", content)

	// First decode quoted-printable if needed
	if strings.Contains(strings.ToLower(content), "=\r\n") || strings.Contains(strings.ToLower(content), "=\n") {
		reader := quotedprintable.NewReader(strings.NewReader(content))
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, reader); err == nil {
			content = buf.String()
		}
	}

	// Split into lines and remove signature line
	lines := strings.Split(content, "\r\n")
	cleanLines := make([]string, 0, len(lines))

	for _, line := range lines {
		// Skip any line containing "Sent from my iPhone"
		if strings.Contains(line, "Sent from my iPhone") {
			continue
		}
		cleanLines = append(cleanLines, line)
	}

	// Join lines back together
	content = strings.Join(cleanLines, "\r\n")

	// Final cleanup of any trailing whitespace or newlines
	content = strings.TrimRight(content, "\r\n \t=")

	log.Printf("Cleaning content (after): %q", content)
	return content
}

// ExtractPlain tries to pull the "best" plain-text representation out of
// whatever you give it:
//
//   - multipart body that starts with "--boundary" lines ➜ first text/plain part
//   - multipart with only text/html           ➜ html→text conversion
//   - anything else (already plain text)      ➜ returned as-is
func ExtractPlain(data []byte) (string, error) {
	trimmed := bytes.TrimSpace(data)

	// quick heuristic: multipart bodies always start with `--something`
	if !bytes.HasPrefix(trimmed, []byte("--")) {
		return string(trimmed), nil // already plain text
	}

	// ── try multipart ───────────────────────────────────────────────
	// find the first line   -->  boundary string
	nl := bytes.IndexByte(trimmed, '\n')
	if nl == -1 {
		return string(trimmed), nil
	}
	boundary := strings.TrimPrefix(
		strings.TrimSpace(string(trimmed[:nl])),
		"--")

	mr := multipart.NewReader(bytes.NewReader(trimmed), boundary)

	var plain, html string
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Not multipart after all → fall back
			return string(trimmed), nil
		}
		ct := p.Header.Get("Content-Type")
		body, _ := io.ReadAll(p)
		switch {
		case strings.HasPrefix(ct, "text/plain"):
			plain = string(body)
		case strings.HasPrefix(ct, "text/html"):
			html = string(body)
		}
	}

	if plain != "" {
		return strings.TrimSpace(plain), nil
	}
	if html != "" {
		txt, err := html2text.FromString(html, html2text.Options{})
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(txt), nil
	}
	// multipart but neither plain nor html found
	return "", nil
}
