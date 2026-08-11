package admin

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/oej/opentea/internal/httpx"
	"github.com/oej/opentea/internal/repo"
)

// auditWrite records an admin_audit_log row via q (the *Repo handed to a
// WithTx closure), so the audit record commits atomically with whatever
// mutation it describes -- spec Sec 22.5: "Authorization-state mutations
// MUST atomically create an immutable audit record and authorization
// outbox event. If either record cannot be committed, the mutation MUST
// fail." previous/resulting are marshaled to JSON snapshots; previous may
// be nil for a create.
func auditWrite(ctx context.Context, q *repo.Repo, actorUUID, requestID, operation, targetType, targetID string, previous, resulting any) error {
	var previousJSON string
	if previous != nil {
		b, err := json.Marshal(previous)
		if err != nil {
			return err
		}
		previousJSON = string(b)
	}
	resultingJSON, err := json.Marshal(resulting)
	if err != nil {
		return err
	}
	return q.RecordAdminAudit(ctx, repo.AdminAuditInput{
		ActorUUID:      actorUUID,
		RequestID:      requestID,
		Operation:      operation,
		TargetType:     targetType,
		TargetID:       targetID,
		PreviousState:  previousJSON,
		ResultingState: string(resultingJSON),
	})
}

// requestIDFrom is a one-line indirection so every audit call site reads
// r.Context() via httpx.RequestID the same way, rather than importing
// httpx directly at each call site.
func requestIDFrom(r *http.Request) string {
	return httpx.RequestID(r.Context())
}
