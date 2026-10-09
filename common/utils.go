package common

import (
	crand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"math/rand"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

func Interface2String(inter interface{}) string {
	switch v := inter.(type) {
	case nil:
		return ""
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case []byte:
		return string(v)
	case json.Number:
		return v.String()
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", inter)
	}
}

func GetUUID() string {
	code := uuid.New().String()
	return strings.ReplaceAll(code, "-", "")
}

const keyChars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func GenerateRandomCharsKey(length int) (string, error) {
	b := make([]byte, length)
	maxI := big.NewInt(int64(len(keyChars)))

	for i := range b {
		n, err := crand.Int(crand.Reader, maxI)
		if err != nil {
			return "", err
		}
		b[i] = keyChars[n.Int64()]
	}

	return string(b), nil
}

func GenerateRandomKey(length int) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("invalid key length: %d", length)
	}
	bytes := make([]byte, length*3/4) // 对于48位的输出，这里应该是36
	if _, err := crand.Read(bytes); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(bytes), nil
}

func GenerateKey() (string, error) {
	//rand.Seed(time.Now().UnixNano())
	return GenerateRandomCharsKey(48)
}

func GetRandomInt(max int) int {
	if max <= 0 {
		return 0
	}
	//rand.Seed(time.Now().UnixNano())
	return rand.Intn(max)
}

func GetTimestamp() int64 {
	return time.Now().Unix()
}

func GetTimeString() string {
	now := time.Now().UTC()
	return fmt.Sprintf("%s%d", now.Format("20060102150405"), now.UnixNano()%1e9)
}

func Max(a int, b int) int {
	if a >= b {
		return a
	} else {
		return b
	}
}

func GetPointer[T any](v T) *T {
	return &v
}

func Any2Type[T any](data any) (T, error) {
	var zero T
	bytes, err := Marshal(data)
	if err != nil {
		return zero, err
	}
	var res T
	err = Unmarshal(bytes, &res)
	if err != nil {
		return zero, err
	}
	return res, nil
}

// BuildURL concatenates base and endpoint, returns the complete url string
func BuildURL(base string, endpoint string) string {
	if base == "" {
		return endpoint
	}
	if endpoint == "" {
		return base
	}
	u, err := url.Parse(base)
	if err != nil {
		return strings.TrimSuffix(base, "/") + "/" + strings.TrimPrefix(endpoint, "/")
	}
	ref, err := url.Parse(endpoint)
	if err != nil {
		return strings.TrimSuffix(base, "/") + "/" + strings.TrimPrefix(endpoint, "/")
	}
	// Absolute endpoint wins.
	if ref.IsAbs() {
		return ref.String()
	}
	return u.ResolveReference(ref).String()
}
