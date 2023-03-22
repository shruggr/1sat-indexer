package lib

import "fmt"

type HttpError struct {
	StatusCode int
	Err        error
}

func (r *HttpError) Error() string {
	return fmt.Sprintf("status %d: error %v", r.StatusCode, r.Err)
}
