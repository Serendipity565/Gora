// Package ijwt 提供基于 HS256 + AES-GCM 的 JWT 令牌管理。
package ijwt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/Serendipity565/gora/configs"
	"github.com/golang-jwt/jwt/v5"
)

type JWT struct {
	signingMethod jwt.SigningMethod // JWT 签名方法
	rcExpiration  time.Duration     // 刷新令牌的过期时间，防止缓存过大
	jwtKey        []byte            // 用于签署 JWT 的密钥
	encKey        []byte            // 用于加密敏感信息的密钥
}

func NewJWT(cfg configs.JWTConfig) *JWT {
	expireDuration, err := time.ParseDuration(cfg.Expire)
	if err != nil {
		// 默认值或返回错误
		expireDuration = 24 * time.Hour
	}
	return &JWT{
		signingMethod: jwt.SigningMethodHS256, //签名的加密方式
		rcExpiration:  expireDuration,
		jwtKey:        []byte(cfg.Secret),
		encKey:        []byte(cfg.EncKey),
	}
}

type UserClaims struct {
	jwt.RegisteredClaims // 内嵌标准的声明
	// 以下是自定义的用户信息字段，均需加密存储在 JWT 中
	UserId string `json:"user_id"`
	Email  string `json:"email"`
}

// SetJWTToken 使用用户 ID 和邮箱生成 JWT 令牌。
func (j *JWT) SetJWTToken(userId uint64, email string) (string, error) {
	userIdEnc, err := j.encryptString(strconv.FormatUint(userId, 10))
	if err != nil {
		return "", fmt.Errorf("userId 加密失败：%w", err)
	}
	emailEnc, err := j.encryptString(email)
	if err != nil {
		return "", fmt.Errorf("password 加密失败：%w", err)
	}
	uc := UserClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(j.rcExpiration)),
		},
		UserId: userIdEnc,
		Email:  emailEnc,
	}

	token := jwt.NewWithClaims(j.signingMethod, uc)
	// 使用指定的secret签名并获得完整的编码后的字符串token
	tokenStr, err := token.SignedString(j.jwtKey)
	if err != nil {
		return "", fmt.Errorf("token 生成失败：%w", err)
	}
	return tokenStr, nil
}

// ParseToken 从请求中提取并返回解析完成的结构体
func (j *JWT) ParseToken(tokenStr string) (UserClaims, error) {
	if j.jwtKey == nil {
		return UserClaims{}, errors.New("jwtKey 为空，无法解析 token")
	}

	//解析token
	uc := UserClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, &uc, func(token *jwt.Token) (interface{}, error) {
		// 校验签名算法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("签名检验算法错误")
		}
		return j.jwtKey, nil
	})
	if err != nil {
		return UserClaims{}, err
	}

	//检查有效性
	if token == nil || !token.Valid {
		return UserClaims{}, errors.New("token无效")
	}

	// 确保 Claims 解析成功
	claims, ok := token.Claims.(*UserClaims)
	if !ok {
		return UserClaims{}, errors.New("无法解析 token claims")
	}

	// 解密敏感信息
	userId, err := j.decryptString(claims.UserId)
	if err != nil {
		return UserClaims{}, fmt.Errorf("userId 解密失败：%w", err)
	}
	email, err := j.decryptString(claims.Email)
	if err != nil {
		return UserClaims{}, fmt.Errorf("password 解密失败：%w", err)
	}
	claims.UserId = userId
	claims.Email = email

	return *claims, nil
}

// 辅助：用 sha256 派生 32 字节 key
func deriveKey(key []byte) []byte {
	h := sha256.Sum256(key)
	return h[:]
}

// encryptString 使用 AES-GCM 将明文加密并返回 base64( nonce | ciphertext )
func (j *JWT) encryptString(plain string) (string, error) {
	key := deriveKey(j.encKey)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nil, nonce, []byte(plain), nil)
	out := append(nonce, ct...)
	return base64.StdEncoding.EncodeToString(out), nil
}

// decryptString 解密 base64( nonce | ciphertext ) 并返回明文
func (j *JWT) decryptString(b64 string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", err
	}
	key := deriveKey(j.encKey)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	ns := gcm.NonceSize()
	if len(data) < ns {
		return "", errors.New("ciphertext too short")
	}
	nonce, ct := data[:ns], data[ns:]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}
