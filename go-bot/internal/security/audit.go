package security

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type AuditLogger struct {
	pool *pgxpool.Pool
}

func NewAuditLogger(pool *pgxpool.Pool) *AuditLogger {
	return &AuditLogger{pool: pool}
}

type AuditEntry struct {
	UserID       int
	CredentialID int
	Action       string
	IPAddress    string
	UserAgent    string
	Success      bool
	ErrorMessage string
}

func (a *AuditLogger) Log(ctx context.Context, entry AuditEntry) error {
	query := `
		INSERT INTO api_key_access_log 
		(user_id, credential_id, action, ip_address, user_agent, success, error_message, accessed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	var ipAddr any
	if entry.IPAddress != "" {
		ipAddr = entry.IPAddress
	}

	_, err := a.pool.Exec(ctx, query,
		entry.UserID,
		entry.CredentialID,
		entry.Action,
		ipAddr,
		entry.UserAgent,
		entry.Success,
		entry.ErrorMessage,
		time.Now(),
	)

	if err != nil {
		return fmt.Errorf("failed to log audit entry: %w", err)
	}

	return nil
}

func (a *AuditLogger) LogDecrypt(ctx context.Context, userID, credentialID int, success bool, errMsg string) error {
	return a.Log(ctx, AuditEntry{
		UserID:       userID,
		CredentialID: credentialID,
		Action:       "decrypt",
		Success:      success,
		ErrorMessage: errMsg,
	})
}

func (a *AuditLogger) LogEncrypt(ctx context.Context, userID, credentialID int, success bool, errMsg string) error {
	return a.Log(ctx, AuditEntry{
		UserID:       userID,
		CredentialID: credentialID,
		Action:       "encrypt",
		Success:      success,
		ErrorMessage: errMsg,
	})
}

func (a *AuditLogger) LogValidation(ctx context.Context, userID, credentialID int, success bool, errMsg string) error {
	return a.Log(ctx, AuditEntry{
		UserID:       userID,
		CredentialID: credentialID,
		Action:       "validate",
		Success:      success,
		ErrorMessage: errMsg,
	})
}
