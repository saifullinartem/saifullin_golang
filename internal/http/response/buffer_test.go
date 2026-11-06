package response_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/saifullinartem/saifullin_golang/internal/http/response"
)

func TestBufferCommit(t *testing.T) {
	buffer := response.NewBuffer()
	buffer.Header().Set("Location", "/trip")
	buffer.WriteHeader(http.StatusCreated)
	buffer.WriteHeader(http.StatusBadRequest)
	buffer.Header().Set("Location", "/late-header")
	for _, chunk := range []string{"first", "second"} {
		if _, err := buffer.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	w := httptest.NewRecorder()
	buffer.Commit(w)
	if w.Code != http.StatusCreated || w.Header().Get("Location") != "/trip" || w.Body.String() != "firstsecond" {
		t.Fatalf("response=%d %v %s", w.Code, w.Header(), w.Body)
	}
}

func TestBufferInvalidStatus(t *testing.T) {
	for _, status := range []int{-1, 1000} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("invalid status did not panic")
				}
			}()
			response.NewBuffer().WriteHeader(status)
		})
	}
}
