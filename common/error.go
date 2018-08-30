package common

import (
	"fmt"
	"net/http"
)

type Error struct {
	Status  int
	Message string
}

func (err *Error) Error() string {
	return err.Message
}

func Errorf(status int, format string, params ...interface{}) error {
	return &Error{
		Status:  status,
		Message: fmt.Sprintf(format, params...),
	}
}

func Status(err error) int {
	if e, ok := err.(*Error); ok {
		return e.Status
	}
	return http.StatusInternalServerError
}
