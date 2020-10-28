package scaleway

import (
	"net/http"

	"github.com/scaleway/scaleway-sdk-go/scw"
)

// is404Error returns true if err is an HTTP 404 error
func is404Error(err error) bool {
	switch newErr := err.(type) {
	case *scw.ResourceNotFoundError:
		return true
	case *scw.ResponseError:
		if newErr.StatusCode == http.StatusNotFound {
			return true
		}
	}

	return false
}
