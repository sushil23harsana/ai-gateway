package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// SemanticResult is a near-duplicate cached response and its cosine distance.
type SemanticResult struct {
	Body        string
	ContentType string
	Model       string
	TokensIn    int
	TokensOut   int
	Distance    float64
}

// vectorLiteral renders a float slice as a pgvector text literal: [1,2,3].
func vectorLiteral(v []float32) string {
	var b strings.Builder
	b.Grow(len(v) * 8)
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(f), 'f', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

// SemanticInsert stores an embedding + its cached response.
func (s *Store) SemanticInsert(ctx context.Context, apiKeyID *string, provider, model string, embedding []float32, body, contentType string, tokensIn, tokensOut int) error {
	const q = `
		INSERT INTO semantic_cache (api_key_id, provider, model, embedding, content_type, body, tokens_in, tokens_out)
		VALUES ($1::uuid, $2, $3, $4::vector, $5, $6, $7, $8)`
	var keyID any
	if apiKeyID != nil {
		keyID = *apiKeyID
	}
	_, err := s.pool.Exec(ctx, q, keyID, provider, model, vectorLiteral(embedding), contentType, body, tokensIn, tokensOut)
	if err != nil {
		return fmt.Errorf("semantic insert: %w", err)
	}
	return nil
}

// SemanticNearest returns the closest cached response within maxDistance (cosine),
// scoped to provider+model and — when apiKeyID is non-nil — that key. It returns
// (nil, nil) when nothing is within the threshold.
func (s *Store) SemanticNearest(ctx context.Context, apiKeyID *string, provider, model string, embedding []float32, maxDistance float64) (*SemanticResult, error) {
	vec := vectorLiteral(embedding)
	q := `
		SELECT body, content_type, model, tokens_in, tokens_out, (embedding <=> $1::vector) AS distance
		FROM semantic_cache
		WHERE provider = $2 AND model = $3`
	args := []any{vec, provider, model}
	if apiKeyID != nil {
		q += ` AND api_key_id = $4::uuid`
		args = append(args, *apiKeyID)
	}
	q += ` ORDER BY embedding <=> $1::vector LIMIT 1`

	var r SemanticResult
	err := s.pool.QueryRow(ctx, q, args...).
		Scan(&r.Body, &r.ContentType, &r.Model, &r.TokensIn, &r.TokensOut, &r.Distance)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("semantic nearest: %w", err)
	}
	if r.Distance > maxDistance {
		return nil, nil
	}
	return &r, nil
}
