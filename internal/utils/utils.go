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
	"mime/quotedprintable"

	"github.com/golang-jwt/jwt/v5"
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

// ExtractInlineMIMEParts extracts the inline parts from a MIME message body.
// Any part is considered inline unless it has Content-Disposition: attachment.
func ExtractInlineMIMEParts(body, boundary string) []string {
	parts := []string{}
	boundaryMarker := "--" + boundary
	sections := strings.Split(body, boundaryMarker)
	for _, section := range sections {
		section = strings.TrimSpace(section)
		if section == "" || strings.HasSuffix(section, "--") {
			continue
		}
		sectionLower := strings.ToLower(section)
		// Skip attachments
		if strings.Contains(sectionLower, "content-disposition: attachment") {
			continue
		}

		// Check if this part uses quoted-printable encoding
		isQuotedPrintable := strings.Contains(sectionLower, "content-transfer-encoding: quoted-printable")

		// Find the first empty line (headers/body separator)
		var content string
		if idx := strings.Index(section, "\r\n\r\n"); idx != -1 {
			content = section[idx+4:]
		} else if idx := strings.Index(section, "\n\n"); idx != -1 {
			content = section[idx+2:]
		}

		content = strings.TrimSpace(content)

		// Decode quoted-printable if needed
		if isQuotedPrintable {
			reader := quotedprintable.NewReader(strings.NewReader(content))
			var buf bytes.Buffer
			if _, err := io.Copy(&buf, reader); err == nil {
				content = buf.String()
			}
		}

		// Clean up the content
		content = cleanMessageContent(content)

		if content != "" {
			parts = append(parts, content)
		}
	}
	return parts
}
