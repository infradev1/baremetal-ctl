package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

const claimID = "id"

type Service struct {
	secret []byte
}

func NewService(secret string) (*Service, error) {
	if secret == "" {
		return nil, errors.New("secret must not be empty")
	}
	svc := &Service{
		secret: []byte(secret),
	}
	return svc, nil
}

func (s *Service) IssueToken(ctx context.Context, userID string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{ // payload inside the JWT
		claimID: userID,
	})

	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", errors.New("error signing secret")
	}

	return signed, nil
}

func (s *Service) ValidateToken(ctx context.Context, token string) (string, error) {
	t, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		// type assertion
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return "", fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		return "", fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := t.Claims.(jwt.MapClaims) // type assertion since t.Claims is an interface
	if !t.Valid || !ok {
		return "", errors.New("failed to extract claims")
	}

	id, ok := claims[claimID].(string) // type assertion again to go from interface to string
	if !ok {
		return "", errors.New("cannot extract user ID from claims")
	}

	return id, nil
}
