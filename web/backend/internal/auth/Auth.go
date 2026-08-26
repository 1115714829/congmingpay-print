// Package auth 负责密码哈希(bcrypt)与 JWT(HS256)签发/校验。
package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// TokenTTL 是 JWT 有效期(24h)。
const TokenTTL = 24 * time.Hour

// User 是签发/校验令牌携带的身份。
type User struct {
	ID   int64
	Name string
	Role string // admin | operator
}

// HashPassword bcrypt(cost 10)哈希密码。
func HashPassword(p string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
	return string(b), err
}

// VerifyPassword 校验明文与哈希。
func VerifyPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// 哨兵错误。
var (
	ErrInvalidToken = errors.New("无效令牌")
	ErrExpired      = errors.New("令牌过期")
)

// Issue 签发 JWT(payload: uid/username/role/exp)。
func Issue(secret string, u User) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"uid":      u.ID,
		"username": u.Name,
		"role":     u.Role,
		"exp":      now.Add(TokenTTL).Unix(),
		"iat":      now.Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString([]byte(secret))
}

// Parse 校验并解析 JWT。
func Parse(secret, token string) (User, error) {
	var u User
	claims := jwt.MapClaims{}
	tok, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return []byte(secret), nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return u, ErrExpired
		}
		return u, ErrInvalidToken
	}
	if !tok.Valid {
		return u, ErrInvalidToken
	}
	if uid, ok := claims["uid"].(float64); ok {
		u.ID = int64(uid)
	}
	if name, ok := claims["username"].(string); ok {
		u.Name = name
	}
	if role, ok := claims["role"].(string); ok {
		u.Role = role
	}
	return u, nil
}

// Bearer 从 Authorization 头提取 Bearer token;缺失返回空串。
func Bearer(header string) string {
	const p = "Bearer "
	if len(header) > len(p) && header[:len(p)] == p {
		return header[len(p):]
	}
	return ""
}
