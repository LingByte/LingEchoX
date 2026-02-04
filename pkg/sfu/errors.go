package sfu

import (
	"errors"

	errors2 "github.com/LingByte/LingEchoX/pkg/errors"
)

var (
	ErrRoomNotFound          = errors.New("room not found")
	ErrNodeNotFound          = errors.New("node not found")
	ErrStreamNotFound        = errors.New("stream not found")
	ErrInvalidRole           = errors.New("invalid role")
	ErrNodeOverloaded        = errors.New("node is overloaded")
	ErrInsufficientResources = errors.New("insufficient resources")
	ErrInvalidMessage        = errors.New("invalid message")
)

// ErrorAdapter adapts old error types to new error system
var (
	// Map old errors to new error codes
	ErrRoomNotFoundNew          = errors2.NewAppError(errors2.ErrCodeRoomNotFound, "Room not found")
	ErrNodeNotFoundNew          = errors2.NewAppError(errors2.ErrCodeNodeNotFound, "Node not found")
	ErrStreamNotFoundNew        = errors2.NewAppError(errors2.ErrCodeStreamNotFound, "Stream not found")
	ErrInvalidRoleNew           = errors2.NewAppError(errors2.ErrCodeInvalidRole, "Invalid role")
	ErrNodeOverloadedNew        = errors2.NewAppError(errors2.ErrCodeNodeOverloaded, "Node is overloaded")
	ErrInsufficientResourcesNew = errors2.NewAppError(errors2.ErrCodeInsufficientResources, "Insufficient resources")
	ErrInvalidMessageNew        = errors2.NewAppError(errors2.ErrCodeInvalidMessage, "Invalid message")
)

// ToAppError converts old error to new AppError
func ToAppError(err error) *errors2.AppError {
	switch err {
	case ErrRoomNotFound:
		return ErrRoomNotFoundNew
	case ErrNodeNotFound:
		return ErrNodeNotFoundNew
	case ErrStreamNotFound:
		return ErrStreamNotFoundNew
	case ErrInvalidRole:
		return ErrInvalidRoleNew
	case ErrNodeOverloaded:
		return ErrNodeOverloadedNew
	case ErrInsufficientResources:
		return ErrInsufficientResourcesNew
	case ErrInvalidMessage:
		return ErrInvalidMessageNew
	default:
		return errors2.WrapError(errors2.ErrCodeInternal, err)
	}
}
