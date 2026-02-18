package auth

import (
	"testing"
	"time"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestMakeJWT_ValidateJWT_OK(t *testing.T) {
	secret := "super-secret"
	userID := uuid.New()

	token, err := MakeJWT(userID, secret, time.Minute)
	if err != nil {
		t.Fatalf("MakeJWT returned error: %v", err)
	}

	gotID, err := ValidateJWT(token, secret)
	if err != nil {
		t.Fatalf("ValidateJWT returned error: %v", err)
	}

	if gotID != userID {
		t.Fatalf("expected userID %v, got %v", userID, gotID)
	}
}

func TestValidateJWT_WrongSecret(t *testing.T) {
	secret := "super-secret"
	userID := uuid.New()

	token, err := MakeJWT(userID, secret, time.Minute)
	if err != nil {
		t.Fatalf("MakeJWT returned error: %v", err)
	}

	_, err = ValidateJWT(token, "wrong-secret")
	if err == nil {
		t.Fatalf("expected error with wrong secret, got nil")
	}
}

func TestValidateJWT_Expired(t *testing.T) {
	secret := "super-secret"
	userID := uuid.New()

	// Expired in the past.
	token, err := MakeJWT(userID, secret, -time.Minute)
	if err != nil {
		t.Fatalf("MakeJWT returned error: %v", err)
	}

	_, err = ValidateJWT(token, secret)
	if err == nil {
		t.Fatalf("expected error for expired token, got nil")
	}
}

func TestValidateJWT_WrongSigningMethod(t *testing.T) {
	secret := "super-secret"

	claims := jwt.RegisteredClaims{
		Issuer:    "chirpy-access",
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		Subject:   uuid.New().String(),
	}

	// Sign with HS512 on purpose (your ValidateJWT only accepts HS256).
	token := jwt.NewWithClaims(jwt.SigningMethodHS512, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("SignedString returned error: %v", err)
	}

	_, err = ValidateJWT(signed, secret)
	if err == nil {
		t.Fatalf("expected error for wrong signing method, got nil")
	}
}

func TestValidateJWT_MalformedToken(t *testing.T) {
	_, err := ValidateJWT("this.is.not.a.jwt", "secret")
	if err == nil {
		t.Fatalf("expected error for malformed token, got nil")
	}
}

func TestValidateJWT_SubjectNotUUID(t *testing.T) {
	secret := "super-secret"

	claims := jwt.RegisteredClaims{
		Issuer:    "chirpy-access",
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		Subject:   "not-a-uuid",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("SignedString returned error: %v", err)
	}

	_, err = ValidateJWT(signed, secret)
	if err == nil {
		t.Fatalf("expected error for non-uuid subject, got nil")
	}
}

func TestValidateJWT_TamperedToken(t *testing.T) {
	secret := "super-secret"
	userID := uuid.New()

	token, err := MakeJWT(userID, secret, time.Minute)
	if err != nil {
		t.Fatalf("MakeJWT returned error: %v", err)
	}

	// Tamper with the token string (change one character). This should break signature.
	tampered := token
	if len(tampered) < 10 {
		t.Fatalf("token unexpectedly short: %q", tampered)
	}
	// Flip one character in the middle in a simple way.
	tampered = tampered[:len(tampered)/2] + "x" + tampered[len(tampered)/2+1:]

	_, err = ValidateJWT(tampered, secret)
	if err == nil {
		t.Fatalf("expected error for tampered token, got nil")
	}
}

func TestGetBearerToken(t *testing.T) {
	tests := []struct {
		name      string
		headerVal string
		wantToken string
		wantErr   bool
	}{
		{
			name:      "missing header",
			headerVal: "",
			wantErr:   true,
		},
		{
			name:      "wrong format - no space",
			headerVal: "Bearer",
			wantErr:   true,
		},
		{
			name:      "wrong scheme",
			headerVal: "Basic abc123",
			wantErr:   true,
		},
		{
			name:      "empty token",
			headerVal: "Bearer ",
			wantErr:   true,
		},
		{
			name:      "valid bearer",
			headerVal: "Bearer abc.def.ghi",
			wantToken: "abc.def.ghi",
			wantErr:   false,
		},
		{
			name:      "lowercase bearer accepted",
			headerVal: "bearer abc.def.ghi",
			wantToken: "abc.def.ghi",
			wantErr:   false,
		},
		{
			name:      "extra spaces are trimmed",
			headerVal: "Bearer    abc.def.ghi   ",
			wantToken: "abc.def.ghi",
			wantErr:   false,
		},
		{
			name:      "token may contain spaces after first split (kept and trimmed)",
			headerVal: "Bearer abc def ghi",
			wantToken: "abc def ghi",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := make(http.Header)
			if tt.headerVal != "" {
				h.Set("Authorization", tt.headerVal)
			}

			got, err := GetBearerToken(h)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (token=%q)", got)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantToken {
				t.Fatalf("expected token %q, got %q", tt.wantToken, got)
			}
		})
	}
}
