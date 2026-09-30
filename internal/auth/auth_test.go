package auth

import (
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestHashing(t *testing.T) {
	pwd := "1234"
	hash, err := HashPassword(pwd)
	if ok, _ := CheckPasswordHash(pwd, hash); err != nil || !ok {
		t.Error("Hashing failed")
	}
}

func TestJWT(t *testing.T) {
	uid := uuid.New()
	secret := "abc"
	jwt, err := MakeJWT(uid, secret, time.Second)
	if u, _ := ValidateJWT(jwt, secret); err != nil || u != uid {
		t.Errorf("JWT %s -> %s failed: %s", uid, u, err)
	}
}
