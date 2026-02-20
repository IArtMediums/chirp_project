package auth
import (
	"github.com/alexedwards/argon2id"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"time"
	"fmt"
	"net/http"
	"strings"
	"crypto/rand"
	"encoding/hex"
)

func HashPassword(password string) (string, error) {
	hash, err := argon2id.CreateHash(password, argon2id.DefaultParams)
	return hash, err
}

func CheckPasswordHash(password, hash string) (bool, error) {
	match, err := argon2id.ComparePasswordAndHash(password, hash)
	return match, err
}

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
	claims := jwt.RegisteredClaims{
		Issuer: "chirpy-access",
		IssuedAt: jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiresIn)),
		Subject: userID.String(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ss, err := token.SignedString([]byte(tokenSecret))
	return ss, err
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	token, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(tokenSecret), nil
	})
	if err != nil {
		return uuid.Nil, err
	} else if claims, ok := token.Claims.(*jwt.RegisteredClaims); ok {
		id, err := uuid.Parse(claims.Subject)
		if err != nil {
			return uuid.Nil, err
		}
		return id, nil
	} else {
		return uuid.Nil, fmt.Errorf("unknown claims type, cannot proceed")
	}
}

func GetBearerToken(headers http.Header) (string, error) {
	auth := headers.Get("Authorization")
	if auth == "" {
		return "", fmt.Errorf("authorization not provided")
	}
	split := strings.SplitN(auth, " ", 2)
	if len(split) != 2 {
		return "", fmt.Errorf("invalid authorization header format")
	}
	if !strings.EqualFold(split[0], "Bearer") {
		return "", fmt.Errorf("invalid authorization scheme: %v", split[0])
	}
	token := strings.TrimSpace(split[1])
	if token == "" {
		return "", fmt.Errorf("token is empty")
	}
	return token, nil
}

func MakeRefreshToken() string {
	key := make([]byte, 32)
	rand.Read(key)
	return hex.EncodeToString(key)
}

func GetAPIKey(headers http.Header) (string, error) {
	auth := headers.Get("Authorization")
	if auth == "" {
		return "", fmt.Errorf("authorization not provided")
	}
	split := strings.SplitN(auth, " ", 2)
	if len(split) != 2 {
		return "", fmt.Errorf("invalid authorization header format")
	}
	if !strings.EqualFold(split[0], "ApiKey") {
		return "", fmt.Errorf("invalid authorization scheme: %v", split [0])
	}
	api_key := strings.TrimSpace(split[1])
	if api_key == "" {
		return "", fmt.Errorf("token is empty")
	}
	return api_key, nil
}
