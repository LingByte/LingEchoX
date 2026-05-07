package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCredentialSign_Vector(t *testing.T) {
	const (
		sk   = "test-secret"
		ts   = "1700000000"
		body = `{}`
	)
	path := CredentialSignPathWithSortedQuery("/api/hello", "b=2&a=1")
	require.Equal(t, "/api/hello?a=1&b=2", path)

	msg := CredentialBuildStringToSign("POST", path, ts, []byte(body))
	sig := CredentialSignHex(sk, msg)
	require.Len(t, sig, 64)
	require.NotEqual(t, CredentialSignHex(sk, msg+"x"), sig)
}
