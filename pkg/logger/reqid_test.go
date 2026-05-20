package logger

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenReqID_NotEmpty(t *testing.T) {
	assert.NotEmpty(t, GenReqID())
}

func TestWithRequestID_ContextRoundTrip(t *testing.T) {
	ctx := WithRequestID(context.Background(), "rid-1")
	assert.Equal(t, "rid-1", RequestIDFromContext(ctx))
}
