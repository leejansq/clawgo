/*
 * Shared utilities for Ad Agents
 */

package agents

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// generateID generates a unique ID for entries
func generateID(prefix string) string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(b))
}

// generateSessionID generates a session ID
func generateSessionID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// formatDate formats a time as date string
func formatDate(t time.Time) string {
	return t.Format("2006-01-02")
}
