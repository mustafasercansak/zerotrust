package auth

import (
	"context"
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestConsumeDPoPProof_RejectsReplay(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	svc := NewService(nil, nil, nil, rdb, nil, nil, nil)
	ctx := context.Background()

	// First use of a jti is accepted.
	if err := svc.ConsumeDPoPProof(ctx, "jti-abc"); err != nil {
		t.Fatalf("first use should succeed, got %v", err)
	}
	// Replay of the same jti is rejected.
	if err := svc.ConsumeDPoPProof(ctx, "jti-abc"); !errors.Is(err, ErrDPoPReplay) {
		t.Fatalf("replay should return ErrDPoPReplay, got %v", err)
	}
	// A distinct jti is accepted.
	if err := svc.ConsumeDPoPProof(ctx, "jti-xyz"); err != nil {
		t.Fatalf("distinct jti should succeed, got %v", err)
	}
}

func TestConsumeDPoPProof_NoRedisOrEmptyJTI_NoOp(t *testing.T) {
	// nil Redis disables the check.
	svc := NewService(nil, nil, nil, nil, nil, nil, nil)
	if err := svc.ConsumeDPoPProof(context.Background(), "anything"); err != nil {
		t.Fatalf("nil redis should be a no-op, got %v", err)
	}

	// Empty jti is a no-op even with Redis present.
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	svc = NewService(nil, nil, nil, rdb, nil, nil, nil)
	if err := svc.ConsumeDPoPProof(context.Background(), ""); err != nil {
		t.Fatalf("empty jti should be a no-op, got %v", err)
	}
}

func TestValidateDPoPProofWithJTI_ReturnsJTI(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	_ = pub
	proof, err := GenerateDPoPProofForTest(priv, "POST", "/api/v1/auth/token")
	if err != nil {
		t.Fatalf("generate proof: %v", err)
	}

	jkt, jti, err := ValidateDPoPProofWithJTI(proof, "POST", "/api/v1/auth/token")
	if err != nil {
		t.Fatalf("validation failed: %v", err)
	}
	if jkt == "" {
		t.Fatal("expected non-empty jkt")
	}
	if jti == "" {
		t.Fatal("expected non-empty jti for replay tracking")
	}
}
